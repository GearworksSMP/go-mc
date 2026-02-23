# Getting Started

Writing automated Minecraft bot programs was one of the original motivations for developing Go-MC. By writing simple client programs and connecting to official servers, we can verify that the other functional modules provided by Go-MC are consistent with the vanilla game. Inherited from Go-MC's predecessor project [gomcbot](https://github.com/Tnze/gomcbot), the `bot` package was the first module developed in Go-MC.

The `bot` package provides a lightweight bot framework that implements features such as getting server status (Ping and List) and connecting to a server to join a game (Join Game). It also includes a simple packet handler registry that uniformly manages received packets, making modular handling of chat, chunks, windows, and other features possible.
