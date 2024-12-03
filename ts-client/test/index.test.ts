// __tests__/client.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { SocketClient, SocketServer, type SocketMessage } from '../src';

// Mock the global WebSocket class
class MockWebSocket {
  public onopen: (() => void) | null = null;
  public onmessage: ((e: MessageEvent) => void) | null = null;
  public onerror: ((e: Event) => void) | null = null;
  public onclose: ((e: CloseEvent) => void) | null = null;
  public readyState: number = WebSocket.CONNECTING;
  
  constructor(public url: string) {}
  
  send(data: string) {
    // No-op for now, or you could capture sent data for assertions
  }

  close() {
    this.readyState = WebSocket.CLOSED;
    if (this.onclose) this.onclose({ code: 1000, reason: 'Normal closure' } as CloseEvent);
  }

  simulateOpen() {
    this.readyState = WebSocket.OPEN;
    if (this.onopen) this.onopen();
  }

  simulateMessage(message: SocketMessage) {
    if (this.onmessage) {
      this.onmessage({ data: JSON.stringify(message) } as MessageEvent);
    }
  }
}

// Install the mock WebSocket globally
globalThis.WebSocket = MockWebSocket as any;

describe('SocketClient', () => {
  it('should connect and subscribe to a channel', () => {
    const client = new SocketClient({ host: 'localhost' });
    const ws = client['socket'] as unknown as MockWebSocket;
    expect(ws.url).toBe('ws://localhost/ws');

    ws.simulateOpen();
    client.subscribe('test-channel');

    expect(client.channel('test-channel')).toBeDefined();
  });

  it('should bind a callback and receive messages', () => {
    const client = new SocketClient({ host: 'localhost' });
    const callback = vi.fn();
    const ws = client['socket'] as unknown as MockWebSocket;

    ws.simulateOpen();

    client.bind('my-channel', 'my-event', callback);

    ws.simulateMessage({
      channel: 'my-channel',
      event: 'my-event',
      payload: { foo: 'bar' }
    });

    expect(callback).toHaveBeenCalledWith({ foo: 'bar' });
  });

  it('should unbind and unsubscribe', () => {
    const client = new SocketClient({ host: 'localhost' });
    const callback = vi.fn();
    const ws = client['socket'] as unknown as MockWebSocket;

    ws.simulateOpen();

    client.bind('some-channel', 'some-event', callback);
    client.unbind('some-channel', 'some-event', callback);
    client.unsubscribe('some-channel');

    expect(client.channel('some-channel')).toBeUndefined();
  });
});

describe('SocketServer', () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({})
      })
    ) as any;
  });

 it('should trigger an event successfully', async () => {
  const server = new SocketServer({ host: 'localhost' });

  await server.trigger({
    channel: 'my-channel',
    event: 'new-message',
    payload: { text: 'hello' },
  });

  expect(globalThis.fetch).toHaveBeenCalledTimes(1);

  // Check the URL and method
  expect(globalThis.fetch).toHaveBeenCalledWith(
    'http://localhost/trigger',
    expect.objectContaining({
      method: 'POST',
    })
  );

  // You can also check that the body and headers are correct
  expect(globalThis.fetch).toHaveBeenCalledWith(
    'http://localhost/trigger',
    expect.objectContaining({
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        channel: 'my-channel',
        event: 'new-message',
        payload: { text: 'hello' },
      }),
    })
  );
});

it('should throw an error if response is not OK', async () => {
  // Instead of casting globalThis.fetch, redefine it as a vi.fn directly
  const mockFetch = vi.fn().mockResolvedValueOnce({
    ok: false,
    statusText: 'Bad Request'
  });

  globalThis.fetch = mockFetch;

  const server = new SocketServer({ host: 'localhost' });

  await expect(
    server.trigger({ channel: 'my-channel', event: 'new-message', payload: { text: 'fail' } })
  ).rejects.toThrowError('Failed to trigger message: Bad Request');

  expect(mockFetch).toHaveBeenCalledTimes(1);
  expect(mockFetch).toHaveBeenCalledWith(
    'http://localhost/trigger',
    expect.objectContaining({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        channel: 'my-channel',
        event: 'new-message',
        payload: { text: 'fail' }
      }),
    })
  );
});});

