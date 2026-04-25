export interface TelegramWebAppUser {
  id: number
  username?: string
}

export interface TelegramWebApp {
  initData: string
  ready(): void
  expand(): void
}

declare global {
  interface Window {
    Telegram?: {
      WebApp?: TelegramWebApp
    }
  }
}

export function getTelegramWebApp() {
  return window.Telegram?.WebApp
}
