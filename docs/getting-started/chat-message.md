# Chat Message

Chat messages are the primary way players communicate in the game. Some servers require players to enter commands to log in after joining. The chat feature can be used not only to report the current status to players in the server but also to receive commands from players -- it is one of the essential features for a bot.

To enable the chat feature for the `bot` package, you need to import the `bot/msg` module, which depends on both `bot/basic` and `bot/playerlist`.

```go
var (
    chatHandler *msg.Manager
    playerList  *playerlist.PlayerList
)


playerList = playerlist.New(client)
chatHandler = msg.New(client, player, playerList, msg.EventsHandler{})
```

## Send Messages

To send a message, call the `SendMessage(msg string) error` method on `msg.Manager`.

```go
if err := chatHandler.SendMessage("Hello, world"); err != nil {
	return err
}
```

## Receive Messages

Currently (1.19.3), there are three types of messages that can be received at the protocol level:
- Player Chat: Messages sent by players, which can be signed to ensure they were sent by the player themselves.
- System Chat: System notifications, such as "Tnze joined the game", displayed either in the chat bar or in the center of the screen.
- Disguised Chat: Messages sent from the server console using the "/say" command.

Here is an example of receiving chat messages:

```go
chatHandler = msg.New(client, player, playerList, msg.EventsHandler{
    SystemChat:        onSystemMsg,
    PlayerChatMessage: onPlayerMsg,
    DisguisedChat:     onDisguisedMsg,
})


func onSystemMsg(msg chat.Message, overlay bool) error {
	log.Printf("System: %v", c)
	return nil
}

func onPlayerMsg(msg chat.Message, validated bool) error {
	log.Printf("Player: %s", msg)
	return nil
}

func onDisguisedMsg(msg chat.Message) error {
	log.Printf("Disguised: %v", msg)
	return nil
}
```

## Handle Message

Minecraft uses a JSON format to store structured chat messages. Messages can include formatting such as color, bold, italic, as well as click events and hover text.

The `go-mc/chat` package provides an API for manipulating chat messages. See [Chat](../chat/README.md) for details.
