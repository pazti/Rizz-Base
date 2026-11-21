export type Page = "home" | "dashboard" | "lobby" | "editor" | "events" | "auth" | "pricing" | "docs" | "studio"
export type Theme = "light" | "dark"

export type Go = (p: Page) => void

export interface PageProps {
  theme: Theme
  toggle: () => void
  go: Go
}

export const repositoryUrl = "https://github.com/pazti/Rizz-Base"
