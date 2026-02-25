package server

import (
	"io"
	"reflect"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/registry"
)

type ConfigHandler interface {
	AcceptConfig(conn *net.Conn) error
}

// KnownPack identifies a data pack by namespace, id, and version.
// The vanilla "minecraft:core" pack uses the game version as its version string.
type KnownPack struct {
	Namespace string
	ID        string
	Version   string
}

func (k KnownPack) WriteTo(w io.Writer) (n int64, err error) {
	n, err = pk.String(k.Namespace).WriteTo(w)
	if err != nil {
		return
	}
	n1, err := pk.String(k.ID).WriteTo(w)
	n += n1
	if err != nil {
		return
	}
	n2, err := pk.String(k.Version).WriteTo(w)
	n += n2
	return
}

func (k *KnownPack) ReadFrom(r io.Reader) (n int64, err error) {
	n, err = (*pk.String)(&k.Namespace).ReadFrom(r)
	if err != nil {
		return
	}
	n1, err := (*pk.String)(&k.ID).ReadFrom(r)
	n += n1
	if err != nil {
		return
	}
	n2, err := (*pk.String)(&k.Version).ReadFrom(r)
	n += n2
	return
}

// RegistryKeys lists entry keys for a single registry type.
type RegistryKeys struct {
	ID      string   // e.g. "minecraft:dimension_type"
	Entries []string // e.g. ["minecraft:overworld", "minecraft:the_nether", ...]
}

// RegistryTagData holds pre-built tag data for a single registry.
type RegistryTagData struct {
	RegistryID string
	Tags       []TagData
}

// TagData is a single tag: a name and list of concrete entry IDs.
type TagData struct {
	Name    string
	Entries []int32
}

type Configurations struct {
	Registries registry.Registries

	// KnownPacks lists data packs the server knows about. When set, the server
	// sends SelectKnownPacks during configuration. If the client recognizes the
	// packs, it uses built-in data for registry entries.
	KnownPacks []KnownPack

	// KnownPackEntries lists registry entry keys for each registry type that
	// should be sent with has_data=false when the client confirms it knows
	// the packs. This tells the client which entries to load from its built-in data.
	KnownPackEntries []RegistryKeys

	// Tags holds registry tag data to send in ClientboundConfigUpdateTags.
	// Tag references (#tag) must already be resolved to concrete entry indices.
	Tags []RegistryTagData
}

// AcceptConfig handles the configuration phase. If KnownPacks is set, it uses
// the SelectKnownPacks exchange to let the client use built-in registry data.
// Otherwise, it sends registry data directly. Then sends FinishConfiguration
// and waits for the client's acknowledgment.
func (c *Configurations) AcceptConfig(conn *net.Conn) error {
	useKnownPacks := false

	// Use SelectKnownPacks to check if the client already has registry data.
	if len(c.KnownPacks) > 0 {
		err := conn.WritePacket(pk.Marshal(
			packetid.ClientboundConfigSelectKnownPacks,
			pk.Array(c.KnownPacks),
		))
		if err != nil {
			return err
		}

		// Read packets until we get the client's SelectKnownPacks response.
		// The client may send ClientInformation, brand, etc. before responding.
		var clientPacks []KnownPack
		var p pk.Packet
		for {
			if err = conn.ReadPacket(&p); err != nil {
				return err
			}
			if packetid.ServerboundPacketID(p.ID) == packetid.ServerboundConfigSelectKnownPacks {
				if err = p.Scan(pk.Array(&clientPacks)); err != nil {
					return err
				}
				break
			}
			// Discard other packets (ClientInformation, CustomPayload, etc.)
		}

		// If the client knows our core pack, it has built-in registry data.
		for _, pack := range clientPacks {
			if pack.Namespace == "minecraft" && pack.ID == "core" {
				useKnownPacks = true
				break
			}
		}
	}

	if useKnownPacks && len(c.KnownPackEntries) > 0 {
		// Send RegistryData with has_data=false for each entry. The client
		// uses its built-in data to fill in the actual values.
		for _, reg := range c.KnownPackEntries {
			if err := writeKeyOnlyRegistryDataPacket(conn, reg.ID, reg.Entries); err != nil {
				return err
			}
		}
	} else {
		// Send full RegistryData from our Registries struct.
		err := c.Registries.EachRegistry(func(registryID string, enc registry.EntryEncoder) error {
			return writeRegistryDataPacket(conn, registryID, enc)
		})
		if err != nil {
			return err
		}
	}

	// Send UpdateTags if tag data is configured.
	if len(c.Tags) > 0 {
		if err := writeUpdateTagsPacket(conn, c.Tags); err != nil {
			return err
		}
	}

	// Send FinishConfiguration
	if err := conn.WritePacket(pk.Marshal(packetid.ClientboundConfigFinishConfiguration)); err != nil {
		return err
	}

	// Read packets until we get the client's FinishConfiguration ack.
	var p pk.Packet
	for {
		if err := conn.ReadPacket(&p); err != nil {
			return err
		}
		if packetid.ServerboundPacketID(p.ID) == packetid.ServerboundConfigFinishConfiguration {
			return nil
		}
		// Discard other packets
	}
}

// writeUpdateTagsPacket sends a ClientboundConfigUpdateTags packet.
func writeUpdateTagsPacket(conn *net.Conn, tags []RegistryTagData) error {
	var pb pk.Builder
	pb.WriteField(pk.VarInt(len(tags)))
	for _, reg := range tags {
		pb.WriteField(pk.Identifier(reg.RegistryID))
		pb.WriteField(pk.VarInt(len(reg.Tags)))
		for _, tag := range reg.Tags {
			pb.WriteField(pk.Identifier(tag.Name))
			pb.WriteField(pk.VarInt(len(tag.Entries)))
			for _, id := range tag.Entries {
				pb.WriteField(pk.VarInt(id))
			}
		}
	}
	return conn.WritePacket(pb.Packet(int32(packetid.ClientboundConfigUpdateTags)))
}

// writeKeyOnlyRegistryDataPacket sends a RegistryData packet with entry keys
// only (has_data=false). The client fills in values from its built-in data pack.
func writeKeyOnlyRegistryDataPacket(conn *net.Conn, registryID string, entries []string) error {
	var pb pk.Builder
	pb.WriteField(pk.Identifier(registryID))
	pb.WriteField(pk.VarInt(len(entries)))
	for _, key := range entries {
		pb.WriteField(pk.Identifier(key))
		pb.WriteField(pk.Boolean(false)) // has_data = false
	}
	return conn.WritePacket(pb.Packet(int32(packetid.ClientboundConfigRegistryData)))
}

// writeRegistryDataPacket sends a single ClientboundConfigRegistryData packet
// for one registry type. The post-1.20.2 format is:
//
//	[Identifier]  registry_id
//	[VarInt]      entry_count
//	Per entry:
//	  [Identifier] entry_id
//	  [Boolean]    has_data
//	  [NBT]        data (if has_data)
func writeRegistryDataPacket(conn *net.Conn, registryID string, enc registry.EntryEncoder) error {
	entries := enc.EncodableEntries()

	var pb pk.Builder
	pb.WriteField(pk.Identifier(registryID))
	pb.WriteField(pk.VarInt(len(entries)))
	for _, entry := range entries {
		pb.WriteField(pk.Identifier(entry.Key))

		// Check if the entry data is a zero/nil value
		hasData := !isNilOrEmpty(entry.Data)
		pb.WriteField(pk.Boolean(hasData))
		if hasData {
			pb.WriteField(pk.NBT(entry.Data))
		}
	}

	return conn.WritePacket(pb.Packet(int32(packetid.ClientboundConfigRegistryData)))
}

// isNilOrEmpty checks if a value is nil or an empty/zero value.
func isNilOrEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Ptr && rv.IsNil()
}

type ConfigFailErr struct {
	reason chat.Message
}

func (c ConfigFailErr) Error() string {
	return "config error: " + c.reason.ClearString()
}
