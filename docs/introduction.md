# Introduction

**Go-MC** is a library written in Go for interacting with Minecraft. Starting as a networking library, it implements MC's TCP-based network protocol in pure Go and provides a `bot` package for conveniently writing command-line MC bot programs.

Go-MC also provides a server framework in the `server` package, and there is a [separate repository](https://github.com/go-mc/server) serving as a server implementation. Server development is not currently covered in this tutorial.

While using the `bot` package, as new features such as sending messages, player movement, and chunk parsing were needed, Go implementations of fundamental game data structures were gradually developed, including *JSON-based chat messages*, *NBT (Named Binary Tag)*, *region storage format (.mca files)*, and more.

- The `net` package implements MC's network transport protocol and data structures such as `VarInt`.
- The `chat` package implements encoding and decoding of the JSON-based chat message format.
- The `level` package implements data structures for representing chunks, BitStorage, palettes, and more.
- The `level/block` package provides block representation data structures and a block state table.
- The `nbt` package implements reflection-based encoding and decoding of the Named Binary Tag format, providing an API similar to the standard library's `json` package.
- The `offline` package provides an algorithm for converting player names to UUIDs in offline mode.
- The `save/region` package implements reading and writing of `.mca` files.
- The `yggdrasil` and `realms` packages are no longer maintained.

When adding these features, both client-side and server-side interfaces were provided. For example, our RCON library includes a server-side implementation. So Go-MC can be used not only to write clients but also to implement servers. The `server` package provides a corresponding framework.

## License

This work is licensed under a [Creative Commons Attribution-ShareAlike 4.0 International License](http://creativecommons.org/licenses/by-sa/4.0/).

[![Creative Commons License](https://i.creativecommons.org/l/by-sa/4.0/88x31.png)](http://creativecommons.org/licenses/by-sa/4.0/)
