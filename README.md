# PushPop

[![jsDocs.io](https://img.shields.io/badge/jsDocs.io-reference-blue)](https://www.jsdocs.io/package/@epklabs/pushpop)

A simple and friendly WebSocket solution for real-time messaging.  
Use the Go server and hub to handle WebSocket communication with clients, and pair it with the TypeScript/JavaScript client library to easily integrate into your frontend applications.

## Features
- **Go Server Binary:** Run a ready-to-use WebSocket server that provides a `/ws` route for connections and a `/trigger` route for sending messages.
- **TypeScript Client:** Connect, subscribe to channels, and bind event handlers in your frontend or Node.js environment.
- **Go Library:** Import the hub and client logic into your own Go application for maximum customization.

## Quick Start

### Running the Go Server via Docker
```bash
docker run -p 8945:8945 ghcr.io/biohackerellie/pushpop:latest
```
This exposes the server on http://localhost:8945 with:
* GET /ws for WebSocket connections
* POST /trigger for sending messages

### Using the TypeScript Client
Install the client from npm:
```bash
npm install @epklabs/pushpop
```
**Server Side Trigger**
```tsx
import {SocketServer} from '@epklabs/pushpop'

const socketServer = new SocketServer({
    host: localhost
    port: 8945
    useTLS: false // true if using HTTPS
})

async function sendMessage() {
    await socketServer.trigger({
        channel: 'my-channel',
        event: 'incoming-message',
        payload: JSON.stringify({message: 'Hello, world!'})
    })
}
```

**Client Side Socket Listener**
```tsx
//src/lib/socket.ts
import {SocketClient} from '@epklabs/pushpop'

// Initialize as a singleton to prevent multiple connections

let socketClient: SocketClient | null = null

export function getSocketClient() {
    if (!socketClient) {
        socketClient = new SocketClient({
            host: 'localhost',
            port: 8945,
            useTLS: false // true if using wss (WebSocket Secure)
        })
    }
    return socketClient
}

//src/app/messages.tsx

import * as React from 'react'
import {getSocketClient} from '../lib/socket'

const Messages: React.FC = () => {
    const [messages, setMessages] = React.useState<string[]>([])
    React.useEffect(() => {
        const socketClient = getSocketClient()

        // PushPop uses channels to group messages together  
        socketClient.subscribe('my-channel')
        // The client requires a handler function to process incoming messages
        const handler = (message: string) => {
            setMessages((prev) => [message, ...prev])
        }

        // Bind the handler to the channel and event
        client.bind('my-channel', 'incoming-message', handler)
        return () => {
            socketClient.unsubscribe('my-channel')
            socketClient.unbind('my-channel', 'incoming-message', handler)
        }
    }, [])
    return (
        <div>
            {messages.map((message, index) => (
                <div key={index}>{message}</div>
            ))}
        </div>
    )
}
```

### Integrating the Go Libary in Your Application
If you prefer to integrate the hub directly into your own Go server:
```go
import(
    "github.com/epklabs/pushpop"
    "net/http"
)

func main() {
    h := pushpop.NewHub()
    go h.Run()

    // pushpop requires only 2 routes: /ws for WebSocket connections and /trigger for sending messages
    http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        pushpop.ServeWs(h, w, r)
    })
    http.HandleFunc("/trigger", pushpop.HandleTrigger(h))

    http.ListenAndServe(":8945", nil)

}
```

You can then trigger messages by using h.Trigger(message) directly in your code.
That's it! With PushPop, you have a lightweight real-time messaging system ready to deploy in Docker, integrate in your backend, or connect to from your frontend.
Feel free to open an issue or contribute if you have ideas or improvements!
