# NBT

Go-MC provides a fully-featured NBT library for conveniently and efficiently handling various NBT structures. This section introduces the core API provided by this NBT library.

To use the NBT library, import the following package:

```go
import "github.com/Tnze/go-mc/nbt"
```

[![Go Reference](https://pkg.go.dev/badge/github.com/Tnze/go-mc/nbt.svg)](https://pkg.go.dev/github.com/Tnze/go-mc/nbt)

The usage of this NBT library is similar to the standard library's `encoding/json` package. Reading the documentation and examples makes it easy to understand. Here are some notes to keep in mind.

Different types of Go variables are converted to different NBT tags. The mapping is as follows:

| Go Type                   | NBT Tag       |
|---------------------------|---------------|
| bool, int8, uint8         | TagByte       |
| int16, uint16             | TagShort      |
| int32, uint32             | TagInt        |
| int64, uint64             | TagLong       |
| float32                   | TagFloat      |
| float64                   | TagDouble     |
| string                    | TagString     |
| struct, map               | TagCompound   |
| []bool, []int8, []uint8   | TagByteArray  |
| []int32, []uint32         | TagIntArray   |
| []int64, []uint64         | TagLongArray  |
| []T                       | TagList       |

The NBT library uses reflection internally to implement these features, but values that implement the `nbt.Marshaler` and `nbt.Unmarshaler` interfaces are not processed via reflection -- the NBT library calls their own implementations instead.

You can support custom types by implementing these two interfaces. The NBT library also has three special types that implement these interfaces, which you can use for special purposes:

- `nbt.RawMessage` -- A "raw data" type where NBT data is stored as-is, capable of holding any valid NBT data.
- [`nbt.StringifiedMessage`](./snbt.md) -- Stores data in S-NBT format and can be converted to/from `string`.
- [`dynbt.Value`](./dynamic-nbt.md) -- A "dynamic" type that can hold any data and provides a convenient API for manipulating NBT.

## Supported Struct Tags and Options

You can tag struct fields to specify how the NBT library encodes them to NBT and decodes NBT into them. The currently supported tags are:

- `nbt` -- Used to specify the name and options
- `nbtkey` -- Used only to specify the name (may contain commas `,`)

### The `nbt` tag

In most cases, you only need this tag to set the NBT tag name.

`nbt` tag format: `<nbt tag>[,opt]`.

This is a comma-separated list. The first item is the tag name; the rest are options.

Like this:
```go
type MyStruct struct {
    Name string `nbt:"name"`
}
```

Use `-` to make the NBT library ignore a field:
```go
type MyStruct struct {
    Internal string `nbt:"-"`
}
```

Use `omitempty` to make the NBT library skip a field only when it has a zero value:
```go
type MyStruct struct {
    Name string `nbt:"name,omitempty"`
}
```

Values of type `[]byte`, `[]int32`, and `[]int64` are encoded as `TagByteArray`, `TagIntArray`, and `TagLongArray` by default, respectively. You can use the `list` option to encode them as `TagList` instead:
```go
type MyStruct struct {
    Data []byte `nbt:"data,list"`
}
```

### The `nbtkey` tag

The official JSON standard library has a significant issue: you cannot specify a key containing a comma (everything after the comma is treated as options) (e.g., `{"a,b" : "c"}`).

The Go team is quite opinionated -- just because they feel keys shouldn't contain commas, they don't let you include commas, even though the JSON standard fully supports it. But we're different. For this use case, we provide a solution:

```go
type MyStruct struct {
    AB string `nbt:",omitempty" nbtkey:"a,b"`
}
```
