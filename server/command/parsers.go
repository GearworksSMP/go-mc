package command

import (
	"io"
	"strconv"
	"strings"

	pk "github.com/Tnze/go-mc/net/packet"
)

type Parser interface {
	Parse(cmd string) (left string, value ParsedData, err error)
}

// Parser IDs for 1.19.3+ (VarInt format, not Identifier).
const (
	parserBrigadierString = 5  // brigadier:string
	parserMinecraftEntity = 6  // minecraft:entity
	parserMinecraftVec3   = 10 // minecraft:vec3
	parserMinecraftMsg    = 20 // minecraft:message
	parserMinecraftGM     = 42 // minecraft:gamemode
)

type StringParser int32

func (s StringParser) WriteTo(w io.Writer) (int64, error) {
	return pk.Tuple{
		pk.VarInt(parserBrigadierString),
		pk.VarInt(s),
	}.WriteTo(w)
}

func (s StringParser) Parse(cmd string) (left string, value ParsedData, err error) {
	switch s {
	case 2: // Greedy Phrase
		return "", cmd, nil
	case 1: // Quotable Phrase
		if len(cmd) > 0 && cmd[0] == '"' {
			var sb strings.Builder
			var isEscaping bool
			for i, v := range cmd[1:] {
				if isEscaping {
					isEscaping = false
					switch v {
					case '\\':
						sb.WriteRune('\\')
					case '"':
						sb.WriteRune('"')
					}
				} else if v == '\\' {
					isEscaping = true
				} else if v == '"' {
					return cmd[:i], sb.String(), nil
				} else {
					sb.WriteRune(v)
				}
			}
			return cmd, nil, ParseErr{
				Pos: len(cmd) - 1,
				Err: "expected '\"'",
			}
		}
		fallthrough
	case 0: // Single Word
		i := strings.IndexAny(cmd, "\t\n\v\f\r ")
		if i == -1 {
			return "", cmd, nil
		}
		return cmd[i:], cmd[:i], nil
	default:
		panic("StringParser: unknown format 0x" + strconv.FormatInt(int64(s), 16))
	}
}

// GamemodeParser is minecraft:gamemode (parser ID 42, no properties).
type GamemodeParser struct{}

func (GamemodeParser) WriteTo(w io.Writer) (int64, error) {
	return pk.VarInt(parserMinecraftGM).WriteTo(w)
}

func (GamemodeParser) Parse(cmd string) (left string, value ParsedData, err error) {
	return StringParser(0).Parse(cmd)
}

// EntityParser is minecraft:entity (parser ID 6, Byte flags).
// Flags: 0x01 = single entity only, 0x02 = players only.
type EntityParser struct {
	Flags byte
}

func (e EntityParser) WriteTo(w io.Writer) (int64, error) {
	return pk.Tuple{
		pk.VarInt(parserMinecraftEntity),
		pk.Byte(e.Flags),
	}.WriteTo(w)
}

func (EntityParser) Parse(cmd string) (left string, value ParsedData, err error) {
	return StringParser(0).Parse(cmd)
}

// Vec3Parser is minecraft:vec3 (parser ID 10, no properties).
type Vec3Parser struct{}

func (Vec3Parser) WriteTo(w io.Writer) (int64, error) {
	return pk.VarInt(parserMinecraftVec3).WriteTo(w)
}

func (Vec3Parser) Parse(cmd string) (left string, value ParsedData, err error) {
	return StringParser(2).Parse(cmd) // greedy
}

// MessageParser is minecraft:message (parser ID 20, no properties).
type MessageParser struct{}

func (MessageParser) WriteTo(w io.Writer) (int64, error) {
	return pk.VarInt(parserMinecraftMsg).WriteTo(w)
}

func (MessageParser) Parse(cmd string) (left string, value ParsedData, err error) {
	return StringParser(2).Parse(cmd) // greedy
}

type ParseErr struct {
	Pos int
	Err string
}

func (p ParseErr) Error() string {
	return p.Err
}
