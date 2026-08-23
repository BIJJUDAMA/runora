package model

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxKVCount        uint64 = 10_000
	MaxTensorCount    uint64 = 100_000
	MaxStringLength   uint64 = 16 * 1024 * 1024 // 16MB
	MaxArrayLength    uint64 = 1_000_000
	MaxRecursionDepth int    = 4
)

type GGUFMetadata struct {
	ID            string
	Name          string
	Architecture  string
	ContextLength uint32
	Quantization  string
	ParamCount    uint64
	FileSize      int64
	FilePath      string
	Layers        uint32
	Heads         uint32
	HeadsKV       uint32
	EmbeddingLen  uint32
	HeadDim       uint32
	Runtime       string
	Task          string
}

// ValueType is the GGUF metadata value type enum
type ValueType uint32

const (
	TypeUInt8   ValueType = 0
	TypeInt8    ValueType = 1
	TypeUInt16  ValueType = 2
	TypeInt16   ValueType = 3
	TypeUInt32  ValueType = 4
	TypeInt32   ValueType = 5
	TypeFloat32 ValueType = 6
	TypeBool    ValueType = 7
	TypeString  ValueType = 8
	TypeArray   ValueType = 9
	TypeUInt64  ValueType = 10
	TypeInt64   ValueType = 11
	TypeFloat64 ValueType = 12
)

// GGUFReader keeps track of parsing state
type GGUFReader struct {
	r       io.Reader
	version uint32
	depth   int
	err     error
}

func (gr *GGUFReader) read(data interface{}) {
	if gr.err != nil {
		return
	}
	gr.err = binary.Read(gr.r, binary.LittleEndian, data)
}

func (gr *GGUFReader) readUint() uint64 {
	if gr.version == 1 {
		var v uint32
		gr.read(&v)
		return uint64(v)
	}
	var v uint64
	gr.read(&v)
	return v
}

func (gr *GGUFReader) readString() string {
	length := gr.readUint()
	if gr.err != nil {
		return ""
	}
	if length > MaxStringLength {
		gr.err = fmt.Errorf("gguf string length %d exceeds max limit %d", length, MaxStringLength)
		return ""
	}
	buf := make([]byte, length)
	_, gr.err = io.ReadFull(gr.r, buf)
	if gr.err != nil {
		return ""
	}
	return string(buf)
}

func (gr *GGUFReader) parseValue(valType ValueType) interface{} {
	if gr.err != nil {
		return nil
	}
	switch valType {
	case TypeUInt8:
		var v uint8
		gr.read(&v)
		return v
	case TypeInt8:
		var v int8
		gr.read(&v)
		return v
	case TypeUInt16:
		var v uint16
		gr.read(&v)
		return v
	case TypeInt16:
		var v int16
		gr.read(&v)
		return v
	case TypeUInt32:
		var v uint32
		gr.read(&v)
		return v
	case TypeInt32:
		var v int32
		gr.read(&v)
		return v
	case TypeFloat32:
		var v float32
		gr.read(&v)
		return v
	case TypeBool:
		var v bool
		gr.read(&v)
		return v
	case TypeString:
		return gr.readString()
	case TypeArray:
		if gr.depth >= MaxRecursionDepth {
			gr.err = fmt.Errorf("max recursion depth %d exceeded while parsing array", MaxRecursionDepth)
			return nil
		}
		gr.depth++
		var elemType uint32
		gr.read(&elemType)
		lenVal := gr.readUint()
		if gr.err != nil {
			gr.depth--
			return nil
		}
		if lenVal > MaxArrayLength {
			gr.err = fmt.Errorf("gguf array length %d exceeds max limit %d", lenVal, MaxArrayLength)
			gr.depth--
			return nil
		}
		arr := make([]interface{}, lenVal)
		for i := uint64(0); i < lenVal; i++ {
			if gr.err != nil {
				gr.depth--
				return nil
			}
			arr[i] = gr.parseValue(ValueType(elemType))
		}
		gr.depth--
		return arr
	case TypeUInt64:
		var v uint64
		gr.read(&v)
		return v
	case TypeInt64:
		var v int64
		gr.read(&v)
		return v
	case TypeFloat64:
		var v float64
		gr.read(&v)
		return v
	default:
		gr.err = fmt.Errorf("unknown GGUF value type %d", valType)
		return nil
	}
}

// ParseGGUF opens a GGUF file and parses its metadata.
func ParseGGUF(filePath string) (*GGUFMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	br := bufio.NewReaderSize(file, 64*1024)

	// GGUF magic is 'G' 'G' 'U' 'F' (0x46554747)
	var magic [4]byte
	if _, err := io.ReadFull(br, magic[:]); err != nil {
		return nil, fmt.Errorf("failed to read magic bytes: %w", err)
	}
	if string(magic[:]) != "GGUF" {
		return nil, fmt.Errorf("invalid GGUF magic bytes")
	}

	var version uint32
	if err := binary.Read(br, binary.LittleEndian, &version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}

	if version < 1 || version > 3 {
		return nil, fmt.Errorf("unsupported GGUF version: %d", version)
	}

	gr := &GGUFReader{
		r:       br,
		version: version,
	}

	// Read tensor count and metadata KV count
	tensorCount := gr.readUint()
	kvCount := gr.readUint()
	if gr.err != nil {
		return nil, fmt.Errorf("failed to read counts: %w", gr.err)
	}

	if kvCount > MaxKVCount {
		return nil, fmt.Errorf("gguf kv count %d exceeds safety limit %d", kvCount, MaxKVCount)
	}
	if tensorCount > MaxTensorCount {
		return nil, fmt.Errorf("gguf tensor count %d exceeds safety limit %d", tensorCount, MaxTensorCount)
	}

	meta := &GGUFMetadata{
		FilePath: filePath,
		FileSize: stat.Size(),
	}

	// Parse key-value pairs
	rawKvs := make(map[string]interface{})
	for i := uint64(0); i < kvCount; i++ {
		key := gr.readString()
		var valType uint32
		gr.read(&valType)
		val := gr.parseValue(ValueType(valType))
		if gr.err != nil {
			return nil, fmt.Errorf("error parsing metadata KV pair %q (index %d): %w", key, i, gr.err)
		}
		rawKvs[key] = val
	}

	// Extract standard values
	if name, ok := rawKvs["general.name"].(string); ok {
		meta.Name = name
	} else {
		// Fallback to filename without directory
		meta.Name = filepath.Base(filePath)
	}

	if arch, ok := rawKvs["general.architecture"].(string); ok {
		meta.Architecture = arch
	}

	// Context length and transformer dimensions depend on architecture
	if meta.Architecture != "" {
		ctxKey := fmt.Sprintf("%s.context_length", meta.Architecture)
		if ctx, ok := rawKvs[ctxKey]; ok {
			meta.ContextLength = toUint32(ctx)
		}
		layersKey := fmt.Sprintf("%s.block_count", meta.Architecture)
		if layers, ok := rawKvs[layersKey]; ok {
			meta.Layers = toUint32(layers)
		}
		headsKey := fmt.Sprintf("%s.attention.head_count", meta.Architecture)
		if heads, ok := rawKvs[headsKey]; ok {
			meta.Heads = toUint32(heads)
		}
		headsKVKey := fmt.Sprintf("%s.attention.head_count_kv", meta.Architecture)
		if headsKV, ok := rawKvs[headsKVKey]; ok {
			meta.HeadsKV = toUint32(headsKV)
		}
		embedKey := fmt.Sprintf("%s.embedding_length", meta.Architecture)
		if embed, ok := rawKvs[embedKey]; ok {
			meta.EmbeddingLen = toUint32(embed)
		}
		headDimKey := fmt.Sprintf("%s.attention.key_length", meta.Architecture)
		if hd, ok := rawKvs[headDimKey]; ok {
			meta.HeadDim = toUint32(hd)
		}
	}

	// Fallback suffix matching for architectures that mismatch general.architecture
	if meta.ContextLength == 0 {
		for k, v := range rawKvs {
			if strings.HasSuffix(k, ".context_length") {
				meta.ContextLength = toUint32(v)
				break
			}
		}
	}
	if meta.Layers == 0 {
		for k, v := range rawKvs {
			if strings.HasSuffix(k, ".block_count") {
				meta.Layers = toUint32(v)
				break
			}
		}
	}
	if meta.Heads == 0 {
		for k, v := range rawKvs {
			if strings.HasSuffix(k, ".attention.head_count") {
				meta.Heads = toUint32(v)
				break
			}
		}
	}
	if meta.HeadsKV == 0 {
		for k, v := range rawKvs {
			if strings.HasSuffix(k, ".attention.head_count_kv") {
				meta.HeadsKV = toUint32(v)
				break
			}
		}
	}
	if meta.EmbeddingLen == 0 {
		for k, v := range rawKvs {
			if strings.HasSuffix(k, ".embedding_length") {
				meta.EmbeddingLen = toUint32(v)
				break
			}
		}
	}
	if meta.HeadDim == 0 {
		for k, v := range rawKvs {
			if strings.HasSuffix(k, ".attention.key_length") || strings.HasSuffix(k, ".attention.head_dim") {
				meta.HeadDim = toUint32(v)
				break
			}
		}
	}

	// Default HeadsKV to Heads if not explicitly defined
	if meta.HeadsKV == 0 {
		meta.HeadsKV = meta.Heads
	}

	// Param count
	if pc, ok := rawKvs["general.parameter_count"]; ok {
		meta.ParamCount = toUint64(pc)
	}

	// File type / Quantization
	if ft, ok := rawKvs["general.file_type"]; ok {
		meta.Quantization = FileTypeToString(toUint32(ft))
	} else if qv, ok := rawKvs["general.quantization_version"]; ok {
		meta.Quantization = fmt.Sprintf("V%d", toUint32(qv))
	} else {
		meta.Quantization = "Unknown"
	}

	// If ParamCount is 0, compute from tensor dimensions
	if meta.ParamCount == 0 {
		var totalParams uint64
		for i := uint64(0); i < tensorCount; i++ {
			_ = gr.readString() // Skip tensor name
			var nDims uint32
			gr.read(&nDims)
			var tensorParams uint64 = 1
			for d := uint32(0); d < nDims; d++ {
				tensorParams *= gr.readUint()
			}
			var tType uint32
			gr.read(&tType)
			_ = gr.readUint() // Skip offset
			if gr.err != nil {
				break
			}
			totalParams += tensorParams
		}
		if gr.err == nil {
			meta.ParamCount = totalParams
		}
	}

	meta.ID = filepath.Base(filePath)
	meta.Runtime = "llama.cpp"
	if meta.EmbeddingLen > 0 {
		meta.Task = "EMBEDDING"
	} else {
		meta.Task = "TEXT_GENERATION"
	}

	return meta, nil
}

func toUint32(v interface{}) uint32 {
	switch val := v.(type) {
	case uint8:
		return uint32(val)
	case int8:
		return uint32(val)
	case uint16:
		return uint32(val)
	case int16:
		return uint32(val)
	case uint32:
		return val
	case int32:
		return uint32(val)
	case uint64:
		return uint32(val)
	case int64:
		return uint32(val)
	case []interface{}:
		if len(val) == 0 {
			return 0
		}
		var sum uint64
		for _, item := range val {
			sum += uint64(toUint32(item))
		}
		// Round to nearest integer to avoid underestimation
		return uint32((float64(sum) / float64(len(val))) + 0.5)
	}
	return 0
}

func toUint64(v interface{}) uint64 {
	switch val := v.(type) {
	case uint8:
		return uint64(val)
	case int8:
		return uint64(val)
	case uint16:
		return uint64(val)
	case int16:
		return uint64(val)
	case uint32:
		return uint64(val)
	case int32:
		return uint64(val)
	case uint64:
		return val
	case int64:
		return uint64(val)
	}
	return 0
}

func FileTypeToString(ft uint32) string {
	switch ft {
	case 0:
		return "F32"
	case 1:
		return "F16"
	case 2:
		return "Q4_0"
	case 3:
		return "Q4_1"
	case 7:
		return "Q8_0"
	case 8:
		return "Q5_0"
	case 9:
		return "Q5_1"
	case 10:
		return "Q2_K"
	case 11:
		return "Q3_K_S"
	case 12:
		return "Q3_K_M"
	case 13:
		return "Q3_K_L"
	case 14:
		return "Q4_K_S"
	case 15:
		return "Q4_K_M"
	case 16:
		return "Q5_K_S"
	case 17:
		return "Q5_K_M"
	case 18:
		return "Q6_K"
	case 19:
		return "IQ2_XXS"
	case 20:
		return "IQ2_XS"
	case 21:
		return "IQ3_XXS"
	case 22:
		return "IQ1_S"
	case 23:
		return "IQ4_NL"
	case 24:
		return "IQ3_S"
	case 25:
		return "IQ2_S"
	case 26:
		return "IQ4_XS"
	case 27:
		return "IQ1_M"
	case 28:
		return "BF16"
	case 29:
		return "Q4_0_XS"
	default:
		return fmt.Sprintf("UNKNOWN (%d)", ft)
	}
}
