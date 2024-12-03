/**
 * PushPop Client Package
 *
 * This package provides a simple and lightweight client library for establishing
 * WebSocket connections, subscribing to channels, and receiving real-time messages
 * from the PushPop server.
 *
 * @remarks
 * With this client, you can:
 * - Connect to a PushPop-compatible WebSocket server.
 * - Subscribe and unsubscribe from channels.
 * - Bind callbacks to events and receive payloads in real time.
 *
 * @example
 * ```typescript
 * import { SocketClient } from '@epklabs/pushpop';
 *
 * const client = new SocketClient({ host: 'localhost', port: '8945', useTLS: false });
 *
 * // Subscribe to a channel
 * client.subscribe('my-channel');
 *
 * // Bind an event and receive messages
 * client.bind('my-channel', 'new-message', (data) => {
 *   console.log('Received:', data);
 * });
 * ```
 * @example
 * Using the `SocketServer` to trigger events:
 * ```typescript
 * import { SocketServer } from '@epklabs/pushpop';
 *
 * const server = new SocketServer({ host: 'localhost', port: '8945', useTLS: false });
 *
 * // Trigger a message on 'my-channel'
 * server.trigger({
 *   channel: 'my-channel',
 *   event: 'new-message',
 *   payload: { text: 'Hello World!' }
 * });
 * ```
 *
 * @see {@link https://github.com/epklabs/pushpop | GitHub repository} for more examples and documentation.
 *
 */

/**
 * Configuration options for setting up the WebSocket connection.
 */
export interface SocketOptions {
  /** Hostname or IP address of the WebSocket server */
  host: string;
  /** Optional port number */
  port?: string;
  /** Optional initial channels to subscribe to */
  channels?: string | string[];
  /** Whether to use TLS (wss/https) or not */
  useTLS?: boolean;
}

/**
 * Interface representing a message sent over the WebSocket.
 */
export interface SocketMessage<T = any> {
  /** The channel name */
  channel: string;
  /** The event name */
  event: string;
  /** The message payload */
  payload: T;
}

/**
 * Class representing a WebSocket server for triggering messages.
 */
export class SocketServer {
  private host: string;
  private port?: string;
  private useTLS?: boolean;

  /**
   * Constructs a new SocketServer instance.
   * @param opts @type SocketOptions
   */
  constructor(opts: SocketOptions) {
    this.host = opts.host;
    this.port = opts.port;
    this.useTLS = opts.useTLS;
  }

  /**
   * Triggers an event to be sent to the server.
   * @param message The message to be sent.
   * @throws Will throw an error if the server response is not OK.
   */
  async trigger<T>(message: SocketMessage<T>): Promise<void> {
    const protocol = this.useTLS ? 'https' : 'http';
    const url = this.port
      ? `${protocol}://${this.host}:${this.port}/trigger`
      : `${protocol}://${this.host}/trigger`;

    const response = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(message),
    });

    if (!response.ok) {
      throw new Error(`Failed to trigger message: ${response.statusText}`);
    }
  }
}

/**
 * Class representing a WebSocket client for subscribing to channels and receiving messages.
 */
export class SocketClient {
  private host: string;
  private port?: string;
  private useTLS?: boolean;
  private socket: WebSocket | null = null;
  private channels: Record<string, Channel> = {};
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;

  /**
   * Constructs a new SocketClient instance and initiates connection.
   * @param opts Configuration options for the client connection.
   */
  constructor(opts: SocketOptions) {
    this.host = opts.host;
    this.port = opts.port;
    this.useTLS = opts.useTLS;
    this.connect();
  }

  /**
   * Retrieves a subscribed channel by name.
   * @param channelName The name of the channel.
   * @returns The Channel instance or undefined if not subscribed.
   */
  channel(channelName: string): Channel | undefined {
    return this.channels[channelName];
  }

  /**
   * Initiates the WebSocket connection and sets up event handlers.
   */
  private connect() {
    const protocol = this.useTLS ? 'wss' : 'ws';
    const socketUrl = this.port
      ? `${protocol}://${this.host}:${this.port}/ws`
      : `${protocol}://${this.host}/ws`;

    this.socket = new WebSocket(socketUrl);

    this.socket.onopen = () => {
      this.reconnectAttempts = 0;
      // Resubscribe to all channels upon reconnection
      Object.keys(this.channels).forEach((channelName) => {
        this.send({
          action: 'subscribe',
          channel: channelName,
        });
      });
    };

    this.socket.onclose = (event) => {
      console.warn(`WebSocket closed: ${event.code} - ${event.reason}`);
      if (this.reconnectAttempts < this.maxReconnectAttempts) {
        console.warn('Attempting to reconnect...');
        const delay = Math.min(1000 * 2 ** this.reconnectAttempts, 30000);
        setTimeout(() => this.connect(), delay);
        this.reconnectAttempts++;
      } else {
        console.error('Max reconnect attempts reached. Giving up.');
      }
    };

    this.socket.onmessage = (event) => {
      const message: SocketMessage = JSON.parse(event.data);
      const channel = this.channels[message.channel];
      if (channel) {
        channel.trigger(message.event, message.payload);
      }
    };

    this.socket.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
  }

  /**
   * Sends data over the WebSocket connection.
   * @param data The data to send.
   */
  private send(data: any) {
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(data));
    } else {
      console.warn('WebSocket is not open. Unable to send message.');
    }
  }

  /**
   * Subscribes to a channel.
   * @param channelName The name of the channel to subscribe to.
   * @returns The Channel instance.
   */
  subscribe(channelName: string): Channel {
    if (!this.channels[channelName]) {
      this.channels[channelName] = new Channel(channelName);
      if (this.socket && this.socket.readyState === WebSocket.OPEN) {
        this.send({
          action: 'subscribe',
          channel: channelName,
        });
      }
    }
    return this.channels[channelName];
  }

  /**
   * Unsubscribes from a channel.
   * @param channelName The name of the channel to unsubscribe from.
   */
  unsubscribe(channelName: string) {
    if (this.channels[channelName]) {
      if (this.socket && this.socket.readyState === WebSocket.OPEN) {
        this.send({
          action: 'unsubscribe',
          channel: channelName,
        });
      }
      delete this.channels[channelName];
    }
  }

  /**
   * Binds a callback function to an event on a channel.
   * @param channelName The name of the channel.
   * @param eventName The name of the event.
   * @param callback The callback function to execute when the event is triggered.
   */
  bind<T>(channelName: string, eventName: string, callback: (data: T) => void) {
    const channel = this.subscribe(channelName);
    channel.bind(eventName, callback);
  }

  /**
   * Unbinds a callback function from an event on a channel.
   * @param channelName The name of the channel.
   * @param eventName The name of the event (optional).
   * @param callback The callback function to remove (optional).
   */
  unbind(channelName: string, eventName?: string, callback?: Function) {
    const channel = this.channels[channelName];
    if (channel) {
      channel.unbind(eventName, callback);
      if (channel.isEmpty()) {
        this.unsubscribe(channelName); // Remove the channel if it has no events bound
      }
    }
  }

  /**
   * Closes the WebSocket connection.
   */
  close() {
    if (this.socket) {
      this.socket.close();
    }
  }
}

/**
 * Class representing a channel to which events can be bound.
 */
class Channel {
  name: string;
  private events: Record<string, Set<Function>> = {};

  /**
   * Constructs a new Channel instance.
   * @param name The name of the channel.
   */
  constructor(name: string) {
    this.name = name;
    this.events = {};
  }

  /**
   * Binds a callback function to an event.
   * @param eventName The name of the event.
   * @param callback The callback function to execute when the event is triggered.
   */
  bind<T>(eventName: string, callback: (data: T) => void) {
    if (!this.events[eventName]) {
      this.events[eventName] = new Set();
    }
    this.events[eventName].add(callback);
  }

  /**
   * Unbinds a callback function from an event.
   * @param eventName The name of the event (optional).
   * @param callback The callback function to remove (optional).
   */
  unbind(eventName?: string, callback?: Function) {
    if (eventName && callback) {
      this.events[eventName]?.delete(callback);
    } else if (eventName) {
      delete this.events[eventName];
    } else {
      this.events = {}; // Clear all events
    }
  }

  /**
   * Triggers an event, executing all bound callback functions.
   * @param eventName The name of the event.
   * @param data The data to pass to the callback functions.
   */
  trigger<T>(eventName: string, data: T) {
    this.events[eventName]?.forEach((callback) => {
      callback(data);
    });
  }

  /**
   * Checks if the channel has any events bound.
   * @returns True if no events are bound, false otherwise.
   */
  isEmpty(): boolean {
    return Object.keys(this.events).length === 0;
  }
}
