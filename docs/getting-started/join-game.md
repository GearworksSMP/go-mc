# Join Game

Although it may seem simple, the process of a player connecting to a server and entering the game is actually somewhat complex. After the client and server successfully establish a connection, they must go through handshaking, authentication, encryption, compression, and several other steps. After a series of packet exchanges, the login phase ends with a [0x02 packet](https://wiki.vg/Protocol#Login_Success) sent from the server to the client, and the player officially begins the game.

Handling login is a bit tedious, but fortunately the `bot` package can handle the entire process for you. All you need to do is call the `JoinServer` function to successfully join the game!

First, let's quickly look at what a basic bot program looks like:

```go
package main

import (
	"log"

	"github.com/Tnze/go-mc/bot"
)

func main() {
	client := bot.NewClient()
	// ...

	err := client.JoinServer("localhost:25565")
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Login success")

	if err = client.HandleGame(); err != nil {
		log.Fatal(err)
	}
}
```

## Step 1: Create the Client

Calling `bot.NewClient()` returns a `*bot.Client` client object representing a game client. There's nothing special to note, but a single client can only connect to one server at a time -- you cannot share a single client across multiple goroutines to connect to servers.

After the player joins the game, the client object will still be used, so you generally save it in a variable for later use. The variable is typically named `client` or `c`.

```go
client := bot.NewClient()
```

## Step 2: Set the Player Name

Changing the player name is fairly straightforward, but be careful not to confuse it with `client.Name`. The authentication information in `client.Auth` is what gets sent to the server during login. `client.Name` is what the server sends back after a successful login.

```go
client.Auth.Name = "Tnze"
```

## Step 3: Log In with a Premium Account

If you want to join a server with online-mode enabled, you also need to provide the **UUID** and **AccessToken**.

```go
client.Auth = bot.Auth{
    Name: "Tnze",
    UUID: "58f6356e-b30c-4811-8bfc-d72a9ee99e73",
    AsTk: "*******************",
}
```

The `yggdrasil` package that used to be included in Go-MC could obtain the AccessToken directly, but after the switch to Microsoft accounts, this became more complicated[^Issue #106] and seems to require a browser or WebView.

Integrating these into Go-MC is not appropriate, so please use community-maintained helper libraries. Known libraries include (in no particular order -- please evaluate and choose for yourself), and new libraries are welcome to be added to this list:

- <https://github.com/maxsupermanhd/go-mc-ms-auth>
- <https://github.com/BaiMeow/msauth>

[^Issue #106]: Issue [#106](https://github.com/Tnze/go-mc/issues/106)

## Step 4: Join the Game

Call `JoinServer()` and pass in the server address to join the game. The server address can be a domain name or an IP address, with or without a port number (default is 25565). By design, this address string is consistent with the address input field in the vanilla game client.

```go
err := client.JoinServer("localhost:25565")
if err != nil {
    log.Fatal(err)
}
```

## Step 5: Handle the Game

After successfully logging into the game, the `JoinServer()` method returns. If you do nothing, the program will exit, the connection will be closed, and the bot won't serve its purpose. So at the end, you need to call the `HandleGame()` method on the client to start sending and receiving game packets:

```go
if err := client.HandleGame(); err != nil {
    log.Fatal(err)
}
```

This method continuously receives packets and executes the corresponding handler functions based on the packet ID. When any handler function produces an error, `HandleGame()` returns. If you don't want the program to exit at that point, you can call `HandleGame()` again after handling the error to continue the game. Calling it multiple times is fine -- this is by design, no need to worry.

```go
var err error
for {
    if err = client.HandleGame(); err == nil {
        panic("HandleGame never return nil")
    }
	log.Printf(err)
}
```

When `HandleGame()` returns an error, you need to distinguish whether the error is recoverable. If it's an unrecoverable error like a disconnection, you should stop the program. If it's just a single packet processing error, you can recover. The way to determine whether the returned error is recoverable is to use the `errors.As()` function to check if `err` is a `bot.PacketHandlerError`:

```diff
  var err error
  var perr bot.PacketHandlerError
  for {
      if err = client.HandleGame(); err == nil {
          panic("HandleGame never return nil")
      }
+     if errors.As(err, &perr) {
+         log.Print(perr)
+     } else {
+         log.Fatal(err)
+     }
  }
```

## The Next Step

Due to the server's "KeepAlive" mechanism, the current bot program does not respond to the server's heartbeat packets and will be kicked after a certain period for being unresponsive. To solve this problem, you need to use the `bot/basic` module covered in the next chapter.
