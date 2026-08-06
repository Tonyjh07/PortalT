// guacamole-common-js 未内置类型声明，此文件提供 RemoteDesktop 组件所需的最小类型面。
declare module 'guacamole-common-js' {
  namespace Guacamole {
    class Client {
      constructor(tunnel: Tunnel)
      connect(data?: string): void
      disconnect(): void
      getDisplay(): Display
      sendMouseState(state: MouseState, sync?: boolean): void
      sendKeyEvent(pressed: 0 | 1, keysym: number): void
      sendSize(width: number, height: number): void
      onerror: (error: { message?: string }) => void
      onstatechange: (state: number) => void
      static State: {
        IDLE: number
        CONNECTING: number
        WAITING: number
        CONNECTED: number
        DISCONNECTING: number
        DISCONNECTED: number
      }
    }

    class Tunnel {
      static State: {
        CONNECTING: number
        CONNECTED: number
        CLOSED: number
        UNSTABLE: number
      }
    }

    class WebSocketTunnel extends Tunnel {
      constructor(tunnelURL: string, receiveTimeout?: number, token?: string)
      oninstruction: ((opcode: string, parameters: string[]) => void) | null
    }

    class Layer {}

    class Display {
      getElement(): HTMLElement
      getWidth(): number
      getHeight(): number
      scale(scale: number): void
      getScale(): number
      draw(layer: Layer, x: number, y: number, url: string): void
      drawImage: (...args: unknown[]) => void
      drawBlob: (...args: unknown[]) => void
    }

    class Mouse {
      constructor(element: HTMLElement)
      onmousedown: (state: MouseState) => void
      onmouseup: (state: MouseState) => void
      onmousemove: (state: MouseState) => void
    }

    interface MouseState {
      x: number
      y: number
      left: boolean
      middle: boolean
      right: boolean
      up: boolean
      down: boolean
    }

    class Keyboard {
      constructor(element: HTMLElement | Document)
      onkeydown: (keysym: number) => void
      onkeyup: (keysym: number) => void
    }
  }

  export = Guacamole
}
