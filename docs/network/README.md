# Network

> This chapter assumes the reader understands how the TCP protocol works and has some experience using it.
> For detailed information about the Minecraft network protocol, see <https://wiki.vg/Protocol>.

As is well known, Minecraft Java Edition uses a TCP-based network transport protocol. TCP has characteristics such as point-to-point communication, reliable transmission, guaranteed ordering, and no preservation of message boundaries. Except for message boundaries, these characteristics also apply to the Minecraft protocol -- keep this in mind when writing code.

On top of TCP, Minecraft builds length-based framing, zlib-based compression, and RSA + AES/CFB8-based encryption protocols.

To send and receive network packets, import the networking library provided by Go-MC:

```go
import (
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
)
```

In special cases where you also need the standard library's `net`, set an alias for Go-MC's networking library:

```go
import (
    "net"
    mcnet "github.com/Tnze/go-mc/net"
    pk "github.com/Tnze/go-mc/net/packet"
)
```

The `net` package mentioned in this chapter refers to `go-mc/net` unless otherwise specified, not the Go standard library's `net` package.

## Create Connection

Creating a connection with the `go-mc/net` package is very similar to creating a TCP connection with the Go standard library.

### Client

The following code connects to a server running at `localhost:25565`:

```go
conn, err := net.DialMC("localhost:25565")
if err != nil {
    log.Fatal(err)
}
defer conn.Close()
```

### Server

The following code listens for connections on **all local IP addresses on port 25565**:

```go
listener, err := net.ListenMC("0.0.0.0:25565")
if err != nil {
    log.Fatal(err)
}

for {
    conn, err := listener.Accept()
    if err != nil {
        log.Fatal(err)
    }
    go handle(&conn)
}
```

## Sending Packets

To send a packet, you need a network connection `net.Conn(conn)` and a packet `pk.Packet(p)`. Simply call `conn.WritePacket(p)` to send the packet.

For how to construct a `pk.Packet`, see the next section [Packing](packing.md).

```go
err := conn.WritePacket(p)
if err != nil {
	log.Print(err)
	return
}
```

## Receiving Packets

Receiving packets is similar to sending them. You also need a `net.Conn(conn)` and a `pk.Packet(p)`. The difference is that this time the operation reads data from the connection and writes it into the packet.

```go
var p pk.Packet
err := conn.ReadPacket(&p)
if err != nil {
	log.Print(err)
	return
}
```

The reason for passing in `&p` rather than directly returning a new `p` is to allow reuse of `pk.Packet` objects.

When receiving packets in a loop, using a single `pk.Packet` per connection reuses the internal buffer, reducing memory allocations and GC pressure.

```go
var p pk.Packet
for {
	err := conn.ReadPacket(&p)
	if err != nil {
		log.Print(err)
		break
	}
	handle(p) // Note: p is only valid until the next ReadPacket call
}
```
