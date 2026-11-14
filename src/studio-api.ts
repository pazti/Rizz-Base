import type { Yap } from "./studio-types"

export interface StudioProject {
  path: string
  name: string
  createdAt: string
}

export interface StudioLobby {
  id: string
  name: string
  createdAt: string
  yapCount: number
}

export interface StudioBackupInfo {
  name: string
  createdAt: string
  lobbyCount: number
  yapCount: number
}

interface StudioBackend {
  ChooseProjectToOpen(): Promise<string>
  ChooseProjectToCreate(): Promise<string>
  ChooseBackupToImport(): Promise<string>
  ChooseRestoreDestination(): Promise<string>
  ChooseBackupDestination(name: string): Promise<string>
  InspectBackup(path: string): Promise<StudioBackupInfo>
  CreateProject(
    path: string,
    name: string,
    passphrase: string,
  ): Promise<StudioProject>
  OpenProject(path: string, passphrase: string): Promise<StudioProject>
  RestoreProject(
    path: string,
    backupPath: string,
    passphrase: string,
  ): Promise<StudioProject>
  CurrentProject(): Promise<StudioProject | null>
  CloseProject(): Promise<void>
  ListLobbies(): Promise<StudioLobby[]>
  CreateLobby(name: string): Promise<StudioLobby>
  DeleteLobby(lobbyId: string): Promise<void>
  ListYaps(lobbyId: string): Promise<Yap[]>
  CreateYap(lobbyId: string, yapId: string, data: string): Promise<Yap>
  UpdateYap(lobbyId: string, yapId: string, data: string): Promise<Yap>
  DeleteYap(lobbyId: string, yapId: string): Promise<void>
  ExportBackup(path: string): Promise<void>
}

declare global {
  interface Window {
    go?: {
      main?: {
        App?: StudioBackend
      }
    }
  }
}

export function studioBackend(): StudioBackend {
  const backend = window.go?.main?.App
  if (!backend) {
    throw new Error(
      "Rizzbase Studio is a desktop app. Launch it with Wails to access local Project files.",
    )
  }
  return backend
}
