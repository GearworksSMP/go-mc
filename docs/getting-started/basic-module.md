# Basic Module

After the `bot` package implements the most basic login logic, other optional features are placed in sub-packages for on-demand import. One nearly essential sub-package is `bot/basic` -- as you can tell from the name, the features it provides are quite fundamental.

The `bot/basic` package provides the following features:

- KeepAlive -- used by the server to test the connection status; the client will be kicked if it doesn't handle this
- Client Settings -- client settings sent to the server, such as language, main hand, equipment visibility, client name, etc.
- Player Info -- stores the player's entity ID, game mode, etc.
- World Info -- stores server information, including world dimension, view distance, maximum player count, etc.

The entire framework is built in a multi-level modular fashion. The packages under `bot` are not all on the same level -- currently `bot/msg` and `bot/world` both depend on the `bot/basic` package. Dependencies between modules are explicitly specified during initialization, requiring no special attention.

## Usage

Module loading must be completed before calling `JoinServer`.

```diff
  package main

  import (
      "log"

      "github.com/Tnze/go-mc/bot"
  )

  var (
      client *bot.Client
      player *basic.Player
  )

  func main() {
      client = bot.NewClient()

+     player = basic.NewPlayer(client, basic.DefaultSettings, basic.EventsListener{})

      err := client.JoinServer("localhost:25565")
      if err != nil {
          log.Fatal(err)
      }

      log.Println("Login success")

      var perr bot.PacketHandlerError
      for {
          if err = client.HandleGame(); err == nil {
              panic("HandleGame never return nil")
          }
          if errors.As(err, &perr) {
              log.Print(perr)
          } else {
              log.Fatal(err)
          }
      }
  }
```

By calling `basic.NewPlayer()`, `basic` registers the `player` object with the provided `client`. When `client` receives the corresponding packets, it automatically parses them and stores the data inside `player` for reading.

> Now, `client` and `player` are two separate objects. `client` is like the open game application window, while `player` is like a game session after joining a server. The former carries the network connection and client settings; the latter carries the character's state data during gameplay. This is a reasonable design, but it isn't immediately obvious at first.

## Events

The `basic` package also provides several events you can register for, such as **game start**, **health change**, **player death**, etc. Using health change as an example, you can pass in the event handler function when calling `basic.NewPlayer()`:

```go
func main() {
	// ...
    player = basic.NewPlayer(client, basic.DefaultSettings, basic.EventsListener{
        HealthChange: onHealthChange,
    })
	// ...
}

func onHealthChange(health float32) error {
    log.Printf("HealthChanged: %v", health)
    return nil
}
```

A complete example can be found at [go-mc/examples/daze](https://github.com/Tnze/go-mc/tree/master/examples/daze)
