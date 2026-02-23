# Chat

Go-MC provides an API for manipulating chat messages in the `go-mc/chat` package.

```go
import "github.com/Tnze/go-mc/chat"
```

The main structure is `chat.Message`, where each `chat.Message` struct represents a single chat message.

## Usage

This struct implements the `pk.Field` interface and can be used directly as a network packet field for sending and receiving:

```go
func HandleDisguisedChatMessagePacket(p pk.Packet) error {
    var (
        Message chat.Message
        ChatType pk.VarInt
        ChatTypeName chat.Message
        TargetName pk.Option[chat.Message, *chat.Message]
    )
    err := p.Scan(&Message, &ChatType, &ChatTypeName, &TargetName)
    // ...
}
```

The struct can also be converted to/from `[]byte` through the `json` standard library:

```go
func ToJson(msg chat.Message) (data []byte, err error) {
    return json.Marshal(msg)
}

func FromJson(data []byte) (msg chat.Message, err error) {
    err = json.Unmarshal(data, &msg)
    return
}
```

Using the `fmt` library, you can directly output styled message text. The `.String()` method converts styles like colors into [ANSI escape sequences](https://en.wikipedia.org/wiki/ANSI_escape_code). So on most terminals in Unix-like systems, you can output styled chat messages directly with `fmt.Print(msg)`. Note that **for Windows support**, you need to use the [go-colorable](https://github.com/mattn/go-colorable) library.

If you need to convert a `chat.Message` to plain text **without ANSI escape sequences**, you can call the `.ClearString()` method.

## Construction

Chat messages can be categorized by the features they use into three types:

- Normal messages
- Extra messages
- Translate messages

### Normal

Normal messages have a simple format, containing only the `.Text` field (without `.Translate`, `.With`, `.Extra`, etc.).

```go
msg := chat.Message {
    Text: "hello, world",
}
```

```
hello, world
```

Normal messages, like other messages, can also include styles:

- Bold
- Italic
- UnderLined
- StrikeThrough
- Obfuscated
- Font
- Color

### Extra

Extra messages append one or more additional messages after the main message. This can be used to apply different styles to different parts of a message.

The appended parts are placed in the `.Extra` field:

```go
msg := chat.Message {
    Text: "hello",
    Extra: []chat.Message {
        chat.Message {
            Text: ", ",
        },
        chat.Message {
            Text: "world",
        },
    },
}
```

```
hello, world
```

### Translate

Translate messages are completely different from normal messages. They **do not contain a `.Text` field**; instead, they use `.Translate` and `.With`.

Translate messages are used to display different messages to clients with different language settings. The principle can be simply understood as a format string (`printf`).

For example, when a player joins a server, the server sends a message with `.Translate == "multiplayer.player.joined"`. In a Chinese client, this reads `"%s joined the game"` (in Chinese), while in an English client it reads `%s joined the game`. The player name is provided through the `.With` field.

Here is a simple example:

```go
msg := chat.Message {
    Trasnlate: "multiplayer.player.joined",
    With: []chat.Message {
        chat.Message {
            Text: "Tnze",
        },
    },
}
```

```
Tnze joined the game
```

Here `Tnze` is filled into the `%s` placeholder of the translate message, resulting in `Tnze joined the game` in an English client.

*All available translation keys can be found under `go-mc/data/lang`.*

> If you want to use Go-MC to implement your own server but also want to provide multi-language support for custom messages, the default translation message list may not be sufficient. You can read the `Locale` field from the `ClientInformation` packet sent by the client to determine the client's language setting, and then perform translation on the server side.

### Shortcuts

Go-MC provides some commonly used constructor functions for quickly generating the message struct you want. I believe these functions are easy to understand and use without additional explanation:

```go
chat.Text("hello, world")
chat.Text("hello").Append(chat.Text(", "), chat.Text("world"))
chat.Text("Warning").SetColor(chat.Yellow)
chat.TranslateMsg("multiplayer.player.joined", chat.Text("Tnze"))
```

## More

Chat messages also support hover events, click events, and other features. For detailed usage, refer to <https://wiki.vg/Chat> and the `go-mc/chat` package source code.
