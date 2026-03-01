package chat

import (
	"bytes"
	"errors"
	"io"
	"strconv"

	"github.com/Tnze/go-mc/nbt"
	pk "github.com/Tnze/go-mc/net/packet"
)

// ReadFrom decode Message in a Text component
func (m *Message) ReadFrom(r io.Reader) (int64, error) {
	return pk.NBT(m).ReadFrom(r)
}

// WriteTo encode Message into a Text component
func (m Message) WriteTo(w io.Writer) (int64, error) {
	return pk.NBT(&m).WriteTo(w)
}

func (m Message) TagType() byte {
	if m.isSimpleText() {
		return nbt.TagString
	}
	return nbt.TagCompound
}

// isSimpleText returns true if the message is plain text with no formatting,
// extras, or translation — suitable for encoding as a bare NBT string.
func (m Message) isSimpleText() bool {
	return m.Translate == "" && m.Color == "" &&
		!m.Bold && !m.Italic && !m.UnderLined && !m.StrikeThrough && !m.Obfuscated &&
		len(m.Extra) == 0 && m.ClickEvent == nil && m.HoverEvent == nil &&
		m.Font == "" && m.Insertion == ""
}

func (m Message) MarshalNBT(w io.Writer) error {
	// Encode value into a buffer using network-format NBT. The encoder writes
	// [tag type byte][payload]. We strip the tag type byte since the outer
	// encoder already wrote it before calling MarshalNBT.
	var buf bytes.Buffer
	enc := nbt.NewEncoder(&buf)
	enc.NetworkFormat(true)

	var err error
	if m.isSimpleText() {
		err = enc.Encode(m.Text, "")
	} else if m.Translate != "" {
		err = enc.Encode(translateMsg(m), "")
	} else {
		err = enc.Encode(rawMsgStruct(m), "")
	}
	if err != nil {
		return err
	}

	data := buf.Bytes()
	if len(data) > 1 {
		_, err = w.Write(data[1:]) // skip the tag type byte
	}
	return err
}

func (m *Message) UnmarshalNBT(tagType byte, r nbt.DecoderReader) error {
	// Re-combine the tagType into the reader, and create a nbt decoder
	tagReader := bytes.NewReader([]byte{tagType})
	decoder := nbt.NewDecoder(io.MultiReader(tagReader, r))
	decoder.NetworkFormat(true) // TagType directlly followed the body

	switch tagType {
	case nbt.TagString:
		_, err := decoder.Decode(&m.Text)
		return err
	case nbt.TagCompound:
		_, err := decoder.Decode((*rawMsgStruct)(m))
		return err
	case nbt.TagList:
		_, err := decoder.Decode(&m.Extra)
		return err
	default:
		return errors.New("unknown chat message type: '" + strconv.FormatUint(uint64(tagType), 16) + "'")
	}
}

func (t *TranslateArgs) UnmarshalNBT(tagType byte, r nbt.DecoderReader) error {
	tagReader := bytes.NewReader([]byte{tagType})
	decoder := nbt.NewDecoder(io.MultiReader(tagReader, r))
	decoder.NetworkFormat(true) // TagType directlly followed the body

	switch tagType {
	case nbt.TagList:
		var value []Message
		if _, err := decoder.Decode(&value); err != nil {
			return err
		}
		for _, v := range value {
			*t = append(*t, v)
		}
		return nil
	case nbt.TagByteArray:
		var value []int8
		if _, err := decoder.Decode(&value); err != nil {
			return err
		}
		for _, v := range value {
			*t = append(*t, strconv.FormatInt(int64(v), 10))
		}
		return nil
	case nbt.TagIntArray:
		var value []int32
		if _, err := decoder.Decode(&value); err != nil {
			return err
		}
		for _, v := range value {
			*t = append(*t, strconv.FormatInt(int64(v), 10))
		}
		return nil
	case nbt.TagLongArray:
		var value []int64
		if _, err := decoder.Decode(&value); err != nil {
			return err
		}
		for _, v := range value {
			*t = append(*t, strconv.FormatInt(int64(v), 10))
		}
		return nil
	default:
		return errors.New("unknown translation args type: '" + strconv.FormatUint(uint64(tagType), 16) + "'")
	}
}
