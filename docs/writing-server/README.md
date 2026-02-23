# Writing Server

Go-MC provides a lightweight server framework that can be used to quickly implement a server.

## Architecture

From a protocol perspective, a player entering a server to play consists of three phases:

|     Phase     | Function                                                                                |
|:-------------:|-----------------------------------------------------------------------------------------|
| Ping & List   | Before the player enters the server, display server information and online player count  |
|     Login     | The player enters the server, handling the login protocol, setting up compression and encryption |
|     Play      | The player has successfully logged in and entered the game, handling game logic and player interaction |

Go-MC is designed around these three phases, providing a highly flexible server framework through interfaces.

The three main interfaces provided by Go-MC are:

- LoginHandler: Provides the login protocol. A default implementation with Mojang authentication is included. Users can write their own LoginHandler to implement custom login authentication.
- ListPingHandler: Provides server status query functionality for external clients. Users can write their own ListPingHandler to display custom server status information.
- GamePlay: The most interesting part, left for users to implement themselves. Users can refer to the version written by Tnze at <https://github.com/go-mc/server>.

//TODO
