# Packing

Once a connection is established, there are three methods you can call on a `net.Conn` instance:

- Send data: `WritePacket(p pk.Packet) error`
- Receive data: `ReadPacket(p *pk.Packet) error`
- Close connection: `Close() error`

> During the Login phase, you also need to call `SetCipher` and `SetThreshold` according to the protocol, but that is not discussed here.

Whether sending or receiving data, the Minecraft protocol processes everything in units of packets (`pk.Packet`). Transport details such as packet length, encryption, and compression[^Packet Format] are already implemented and encapsulated within the `pk.Packet` struct by Go-MC, so users can use it directly.

[^Packet Format]: Packet format: <https://wiki.vg/Protocol#Packet_format>

The `pk.Packet` struct is defined as follows:

```go
type Packet struct {
	ID   int32
	Data []byte
}
```

`ID` is an enumeration value indicating the packet's function, and the format of `Data` must be determined based on it. Available values can be found in `data/packetid`.

`Data` logically contains one or more data fields. For example, the Move packet that a client might send when a player moves could have the following format:

| Field Name | Field Type | Notes                                                 |
|------------|------------|-------------------------------------------------------|
| X          | Double     | Absolute position.                                    |
| Feet Y     | Double     | Absolute feet position, normally Head Y - 1.62.       |
| Z          | Double     | Absolute position.                                    |
| On Ground  | Boolean    | True if the client is on the ground, false otherwise. |

That is, `Data` contains 4 data fields: three Doubles and one Boolean.

All field types are defined in the `net/packet` package, such as `pk.Double` and `pk.Boolean`. These types all implement the `pk.Field` interface, and you can call their `ReadFrom` and `WriteTo` methods.

The following code can be used to generate such a packet:

```go
var buffer bytes.Buffer
_, _ = pk.Double(x).WriteTo(&buffer)
_, _ = pk.Double(y).WriteTo(&buffer)
_, _ = pk.Double(z).WriteTo(&buffer)
_, _ = pk.Boolean(onGround).WriteTo(&buffer)

p := pk.Packet {
	ID: packetid.ServerboundMovePlayerPos,
	Data: buffer.Bytes(),
}
```

This is a bit tedious, but fortunately Go-MC provides a helper function for this: `pk.Marshal()`. The improved code looks like this:

```go
p := pk.Marshal(
    packetid.ServerboundMovePlayerPos,
    pk.Double(x),
    pk.Double(y),
    pk.Double(z),
	pk.Boolean(onGround),
)
```

Conversely, if the server wants to receive such a packet, you need to use the `ReadFrom` method. The straightforward approach looks like this:

```go
var (
	x        pk.Double
	y        pk.Double
	z        pk.Double
	onGround pk.Boolean
)
r := bytes.NewReader(p.Data)
_, _ = x.ReadFrom(r)
_, _ = y.ReadFrom(r)
_, _ = z.ReadFrom(r)
_, _ = onGround.ReadFrom(r)
```

Adding error handling makes it even more tedious! Fortunately, Go-MC also provides a helper function: `p.Scan()`. The improved code looks like this:

```go
var (
    x        pk.Double
    y        pk.Double
    z        pk.Double
    onGround pk.Boolean
)
_ = p.Scan(&x, &y, &z, &onGround)
```

In practice, you should decide whether to use the helper functions based on the situation.

## Using `pk.Array`

In some packet formats, there are `Array of X` type array fields. These arrays are usually preceded by a `VarInt` length field. Go-MC provides a helper type to handle this common case.

Taking the Commands packet as an example, the current packet format is as follows:

| Field Name | Field Type      | Notes                                         |
|------------|-----------------|-----------------------------------------------|
| Count      | VarInt          | Number of elements in the following array.    |
| Nodes      | Array of `Node` | An array of nodes.                            |
| Root index | VarInt          | Index of the root node in the previous array. |

Assuming you have already defined the `Node` type and implemented the `pk.Field` interface for it, to receive the Nodes array in this packet, you could use the following verbose code:

```go
var (
	count pk.VarInt
	nodes []Node
	root  pk.VarInt
)
r := bytes.NewReader(p.Data)

_, _ = count.ReadFrom(r)
nodes = make([]Node, count)
for i := range nodes {
	_, _ = nodes[i].ReadFrom(r)
}
root.ReadFrom(r)
```

To simplify using `p.Scan()`, we can use the `pk.Array()` function:

```go
var (
	nodes []Node
	root  pk.VarInt
)
_ = p.Scan(
	pk.Array(&nodes),
	&root,
)
```

Using `pk.Array()` automatically handles the `VarInt` array length. The return value implements the `pk.Field` interface and can be used with `pk.Marshal()` and `pk.Scan()`.

Note that if the slice's element type does not support the corresponding `WriteTo` and `ReadFrom` methods, it will panic. Make sure that when using the return value of `pk.Array()` as a `FieldEncoder`, the slice element type also implements `FieldEncoder`, and when using it as a `FieldDecoder`, the slice element type also implements `FieldDecoder`.

## Using `pk.Option`

In some packet formats, there are `Optional X` type optional fields. As the name suggests, these fields are optional within the packet. Whether an optional field is present depends on the context, which usually means a preceding Boolean value, for example:

| Field Name | Field Type      | Notes                           |
|------------|-----------------|---------------------------------|
| Is Signed  | Boolean         |                                 |
| Signature  | Optional String | Exist only if Is Signed is true |

Pseudocode for reading this packet:

```go
IsSigned = ReadBoolean()
if IsSigned {
	Signature = ReadString()
}
```

To provide this kind of conditional logic within a `p.Scan()` function call, Go-MC provides four helper types: `pk.Option`, `pk.OptionDecoder`, `pk.OptionEncoder`, and `pk.Opt`.

`pk.Opt` is a primitive type that is rarely used directly. If you need it, please read the comments and source code yourself -- it is not explained here.

`pk.Option` is a generic type with two type parameters: `T` and `P`, where the latter is a pointer type to the former. `T` must satisfy the `pk.FieldEncoder` interface, and `P` must satisfy the `FieldDecoder` interface.

For example: `pk.Option[pk.String, *pk.String]`, `pk.Option[pk.VarInt, *pk.VarInt]`, etc., are all valid types.

Manually specifying `P` is necessary because the current Go compiler's generics implementation is not yet complete enough[^1]. Once Go supports inferring struct generic parameter types, the above two examples could be simplified to `pk.Option[pk.String]` and `pk.Option[pk.VarInt]`.

[^1] [Type Parameters Proposal](https://go.googlesource.com/proposal/+/refs/heads/master/design/43651-type-parameters.md#pointer-method-example)

`pk.OptionDecoder` and `pk.OptionEncoder` are two variants that respectively remove the constraint on `T` and the constraint on `P` -- straightforward enough.

Here is the complete code for reading and writing the example packet above:

```go
var Signature pk.Option[pk.String, *pk.String]
_ = p.Scan(&Signature)
```
