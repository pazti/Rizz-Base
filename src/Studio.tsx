import { useEffect, useRef, useState, type FormEvent } from "react"
import {
  studioBackend,
  type StudioBackupInfo,
  type StudioLobby,
  type StudioProject,
} from "./studio-api"
import type { Yap } from "./studio-types"

type ProjectAction = "open" | "create" | "restore"

const previewProject: StudioProject = {
  path: "~/Documents/rizzbase-demo.rizz",
  name: "Rizzbase Demo",
  createdAt: "2026-11-14T00:00:00Z",
}

const previewLobbies: StudioLobby[] = [
  {
    id: "dev-notes",
    name: "dev-notes",
    createdAt: "2026-11-14T00:00:00Z",
    yapCount: 3,
  },
  {
    id: "project-ideas",
    name: "project-ideas",
    createdAt: "2026-11-14T00:00:00Z",
    yapCount: 2,
  },
  {
    id: "daily-log",
    name: "daily-log",
    createdAt: "2026-11-14T00:00:00Z",
    yapCount: 1,
  },
]

const previewYaps: Yap[] = [
  {
    id: "welcome",
    data: {
      author: "mika",
      content: "just shipped the local-first preview",
      tags: ["studio", "shipping"],
    },
  },
  {
    id: "backup-plan",
    data: {
      author: "jules",
      content: "encrypted project + protected JSON backup",
      secure: true,
    },
  },
  {
    id: "next-step",
    data: {
      author: "kai",
      content: "build something useful, then measure it",
      priority: 1,
    },
  },
]

const previewYapsByLobby: Record<string, Yap[]> = {
  "dev-notes": previewYaps,
  "project-ideas": [
    { id: "desktop-ui", data: { status: "in progress", platform: "Wails" } },
    { id: "backup-flow", data: { format: "JSON", encryptedAtRest: false } },
  ],
  "daily-log": [
    {
      id: "today",
      data: { focus: "ship a useful local-first tool", done: false },
    },
  ],
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function prettyJSON(value: unknown): string {
  return JSON.stringify(value, null, 2)
}

export default function Studio({ preview = false }: { preview?: boolean }) {
  const [project, setProject] = useState<StudioProject | null>(
    preview ? previewProject : null,
  )
  const [lobbies, setLobbies] = useState<StudioLobby[]>(
    preview ? previewLobbies : [],
  )
  const [activeLobbyID, setActiveLobbyID] = useState(
    preview ? (previewLobbies[0]?.id ?? "") : "",
  )
  const [yaps, setYaps] = useState<Yap[]>(preview ? previewYaps : [])
  const [projectAction, setProjectAction] = useState<ProjectAction | null>(null)
  const [projectPath, setProjectPath] = useState("")
  const [backupPath, setBackupPath] = useState("")
  const [backupSummary, setBackupSummary] = useState<StudioBackupInfo | null>(
    null,
  )
  const [projectName, setProjectName] = useState("")
  const [passphrase, setPassphrase] = useState("")
  const [editingYap, setEditingYap] = useState<Yap | null>(null)
  const [isNewYap, setIsNewYap] = useState(false)
  const [yapID, setYapID] = useState("")
  const [yapJSON, setYapJSON] = useState("{}")
  const [newLobbyName, setNewLobbyName] = useState("")
  const [showLobbyForm, setShowLobbyForm] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [notice, setNotice] = useState("")
  const dialogRef = useRef<HTMLFormElement | null>(null)
  const dialogOpenerRef = useRef<HTMLElement | null>(null)
  const busyRef = useRef(false)
  const dialogOpen = Boolean(
    projectAction || showLobbyForm || isNewYap || editingYap,
  )

  function rememberDialogOpener() {
    if (document.activeElement instanceof HTMLElement) {
      dialogOpenerRef.current = document.activeElement
    }
  }

  useEffect(() => {
    if (!dialogOpen) return
    const activeDialog = dialogRef.current
    if (!activeDialog) return

    const containsFocus = activeDialog.contains.bind(activeDialog)
    const focusableSelector =
      'button:not(:disabled), input:not(:disabled), textarea:not(:disabled), select:not(:disabled), [tabindex]:not([tabindex="-1"])'
    const focusable = () =>
      Array.from(activeDialog.querySelectorAll<HTMLElement>(focusableSelector))
    const initialFocus =
      activeDialog.querySelector<HTMLElement>("[autofocus]") ?? focusable()[0]
    initialFocus?.focus()

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault()
        if (busyRef.current) return
        setProjectAction(null)
        setPassphrase("")
        setBackupSummary(null)
        setShowLobbyForm(false)
        setIsNewYap(false)
        setEditingYap(null)
        setError("")
        return
      }
      if (event.key !== "Tab") return

      const items = focusable()
      if (!items.length) {
        event.preventDefault()
        return
      }
      const first = items[0]
      const last = items[items.length - 1]
      if (
        event.shiftKey &&
        (document.activeElement === first ||
          !containsFocus(document.activeElement))
      ) {
        event.preventDefault()
        last.focus()
      } else if (
        !event.shiftKey &&
        (document.activeElement === last ||
          !containsFocus(document.activeElement))
      ) {
        event.preventDefault()
        first.focus()
      }
    }

    document.addEventListener("keydown", onKeyDown)
    return () => {
      document.removeEventListener("keydown", onKeyDown)
      const opener = dialogOpenerRef.current
      if (opener?.isConnected) opener.focus()
      dialogOpenerRef.current = null
    }
  }, [dialogOpen])

  useEffect(() => {
    if (preview) return
    let active = true
    try {
      studioBackend()
        .CurrentProject()
        .then(async (current) => {
          if (!active || !current) return
          setProject(current)
          const rows = await studioBackend().ListLobbies()
          if (!active) return
          setLobbies(rows)
          setActiveLobbyID(rows[0]?.id ?? "")
        })
        .catch((cause: unknown) => {
          if (active) setError(errorMessage(cause))
        })
    } catch (cause) {
      setError(errorMessage(cause))
    }
    return () => {
      active = false
    }
  }, [preview])

  useEffect(() => {
    if (preview) {
      setYaps(previewYapsByLobby[activeLobbyID] ?? [])
      return
    }
    if (!activeLobbyID) {
      setYaps([])
      return
    }
    let active = true
    studioBackend()
      .ListYaps(activeLobbyID)
      .then((rows) => {
        if (active) setYaps(rows)
      })
      .catch((cause: unknown) => {
        if (active) setError(errorMessage(cause))
      })
    return () => {
      active = false
    }
  }, [activeLobbyID, preview])

  async function run(action: () => Promise<void>) {
    busyRef.current = true
    setBusy(true)
    setError("")
    setNotice("")
    try {
      await action()
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      busyRef.current = false
      setBusy(false)
    }
  }

  async function chooseProject(action: ProjectAction) {
    rememberDialogOpener()
    await run(async () => {
      const backend = studioBackend()
      let path = ""
      let selectedBackup = ""
      let selectedBackupSummary: StudioBackupInfo | null = null
      if (action === "open") {
        path = await backend.ChooseProjectToOpen()
      } else if (action === "restore") {
        selectedBackup = await backend.ChooseBackupToImport()
        if (!selectedBackup) return
        selectedBackupSummary = await backend.InspectBackup(selectedBackup)
        path = await backend.ChooseRestoreDestination()
      } else {
        path = await backend.ChooseProjectToCreate()
      }
      if (!path) return
      setProjectPath(path)
      setBackupPath(selectedBackup)
      setBackupSummary(selectedBackupSummary)
      setProjectAction(action)
      setProjectName(action === "create" ? "My Rizzbase Project" : "")
      setPassphrase("")
      setError("")
    })
  }

  async function submitProject(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    await run(async () => {
      const backend = studioBackend()
      let next: StudioProject
      switch (projectAction) {
        case "create":
          next = await backend.CreateProject(
            projectPath,
            projectName,
            passphrase,
          )
          break
        case "restore":
          next = await backend.RestoreProject(
            projectPath,
            backupPath,
            passphrase,
          )
          break
        case "open":
          next = await backend.OpenProject(projectPath, passphrase)
          break
        default:
          throw new Error("No Project action is selected.")
      }
      setPassphrase("")
      setProjectAction(null)
      setBackupSummary(null)
      setProject(next)
      const rows = await backend.ListLobbies()
      setLobbies(rows)
      setActiveLobbyID(rows[0]?.id ?? "")
    })
  }

  async function closeProject() {
    await run(async () => {
      await studioBackend().CloseProject()
      setProject(null)
      setLobbies([])
      setActiveLobbyID("")
      setYaps([])
    })
  }

  async function createLobby(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    await run(async () => {
      const lobby = await studioBackend().CreateLobby(newLobbyName)
      const rows = await studioBackend().ListLobbies()
      setLobbies(rows)
      setActiveLobbyID(lobby.id)
      setNewLobbyName("")
      setShowLobbyForm(false)
      setNotice(`Lobby "${lobby.name}" created.`)
    })
  }

  function startCreateYap() {
    rememberDialogOpener()
    setEditingYap(null)
    setIsNewYap(true)
    setYapID("")
    setYapJSON("{\n  \n}")
    setError("")
  }

  function startEditYap(yap: Yap) {
    rememberDialogOpener()
    setEditingYap(yap)
    setIsNewYap(false)
    setYapID(yap.id)
    setYapJSON(prettyJSON(yap.data))
    setError("")
  }

  async function saveYap(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    await run(async () => {
      const data: unknown = JSON.parse(yapJSON)
      const normalized = prettyJSON(data)
      if (isNewYap) {
        await studioBackend().CreateYap(activeLobbyID, yapID, normalized)
      } else if (editingYap) {
        await studioBackend().UpdateYap(
          activeLobbyID,
          editingYap.id,
          normalized,
        )
      }
      setEditingYap(null)
      setIsNewYap(false)
      setYaps(await studioBackend().ListYaps(activeLobbyID))
      setLobbies(await studioBackend().ListLobbies())
    })
  }

  async function deleteYap(yap: Yap) {
    if (!window.confirm(`Permanently delete Yap "${yap.id}"?`)) return
    await run(async () => {
      await studioBackend().DeleteYap(activeLobbyID, yap.id)
      setYaps(await studioBackend().ListYaps(activeLobbyID))
      setLobbies(await studioBackend().ListLobbies())
    })
  }

  async function deleteLobby(lobby: StudioLobby) {
    if (
      !window.confirm(
        `Delete "${lobby.name}" and all ${lobby.yapCount} Yaps in it?`,
      )
    )
      return
    await run(async () => {
      await studioBackend().DeleteLobby(lobby.id)
      const rows = await studioBackend().ListLobbies()
      setLobbies(rows)
      setActiveLobbyID(rows[0]?.id ?? "")
    })
  }

  async function exportBackup() {
    await run(async () => {
      const safeName = project?.name
        .replace(/[<>:"/\\|?*\u0000-\u001f]/g, "-")
        .slice(0, 120)
      const name = safeName ? `${safeName}-backup.json` : "rizzbase-backup.json"
      const path = await studioBackend().ChooseBackupDestination(name)
      if (!path) return
      await studioBackend().ExportBackup(path)
      setNotice(
        "Encrypted Project data was exported as a decrypted JSON backup. Store it securely.",
      )
    })
  }

  if (!project) {
    return (
      <main className="studio-root studio-start">
        <header className="studio-brand">
          <img
            className="studio-brand-mark"
            src="/brand/rizzbase-mark-light.svg"
            alt=""
          />
          <img
            className="studio-brand-wordmark"
            src="/brand/rizzbase-wordmark-on-light.svg"
            alt="Rizzbase"
          />
          <span className="studio-product-label">STUDIO</span>
        </header>
        <section className="studio-empty-card">
          <span className="studio-eyebrow">LOCAL-FIRST DATABASE WORKSPACE</span>
          <h1>
            Your Projects,
            <br />
            on your machine.
          </h1>
          <p>
            Create or open a Project, or restore a JSON backup into a new
            encrypted file. Nothing is uploaded.
          </p>
          <div className="studio-start-actions">
            <button
              type="button"
              className="studio-button studio-primary"
              disabled={busy}
              onClick={() => void chooseProject("create")}
            >
              CREATE PROJECT
            </button>
            <button
              type="button"
              className="studio-button"
              disabled={busy}
              onClick={() => void chooseProject("open")}
            >
              OPEN PROJECT
            </button>
            <button
              type="button"
              className="studio-button"
              disabled={busy}
              onClick={() => void chooseProject("restore")}
            >
              RESTORE BACKUP
            </button>
          </div>
          <div className="studio-privacy">
            <span>ENCRYPTED BY DEFAULT</span>
            <small>
              Use a strong passphrase you can keep safe. There is no passphrase
              recovery.
            </small>
            <small>
              Questions? <a href="mailto:hello@rizzbase.tech">hello@rizzbase.tech</a>
              {" · "}
              <a href="mailto:paul@rizzbase.tech">paul@rizzbase.tech</a>
            </small>
          </div>
        </section>
        {error && (
          <div className="studio-toast studio-error" role="alert">
            {error}
          </div>
        )}
        {projectAction && (
          <div className="studio-modal-backdrop" role="presentation">
            <form
              ref={dialogRef}
              className="studio-modal"
              role="dialog"
              aria-modal="true"
              aria-labelledby="studio-project-dialog-title"
              tabIndex={-1}
              onSubmit={(event) => void submitProject(event)}
            >
              <button
                className="studio-modal-close"
                type="button"
                aria-label="Close"
                disabled={busy}
                onClick={() => {
                  setProjectAction(null)
                  setPassphrase("")
                  setBackupSummary(null)
                }}
              >
                ×
              </button>
              <span className="studio-eyebrow">
                {projectAction === "create"
                  ? "NEW ENCRYPTED PROJECT"
                  : projectAction === "restore"
                    ? "RESTORE FROM JSON BACKUP"
                    : "OPEN ENCRYPTED PROJECT"}
              </span>
              <h2 id="studio-project-dialog-title">
                {projectAction === "create"
                  ? "Set up a Project."
                  : projectAction === "restore"
                    ? "Encrypt restored Project."
                    : "Unlock your Project."}
              </h2>
              {projectAction === "restore" ? (
                <>
                  <p className="studio-path">JSON backup: {backupPath}</p>
                  <p className="studio-path">New Project: {projectPath}</p>
                  {backupSummary ? (
                    <section
                      className="studio-backup-summary"
                      aria-labelledby="studio-backup-summary-title"
                    >
                      <h3 id="studio-backup-summary-title">BACKUP DETAILS</h3>
                      <dl>
                        <div>
                          <dt>Project</dt>
                          <dd>{backupSummary.name}</dd>
                        </div>
                        <div>
                          <dt>Created</dt>
                          <dd>
                            <time dateTime={backupSummary.createdAt}>
                              {new Date(
                                backupSummary.createdAt,
                              ).toLocaleString()}
                            </time>
                          </dd>
                        </div>
                        <div>
                          <dt>Lobbies</dt>
                          <dd>{backupSummary.lobbyCount}</dd>
                        </div>
                        <div>
                          <dt>Yaps</dt>
                          <dd>{backupSummary.yapCount}</dd>
                        </div>
                      </dl>
                    </section>
                  ) : (
                    <div
                      className="studio-inline-message studio-error"
                      role="alert"
                    >
                      Backup details are unavailable. Select the backup again.
                    </div>
                  )}
                </>
              ) : (
                <p className="studio-path">{projectPath}</p>
              )}
              {error && (
                <div
                  className="studio-inline-message studio-error"
                  role="alert"
                >
                  {error}
                </div>
              )}
              {projectAction === "create" && (
                <label>
                  PROJECT NAME
                  <input
                    autoFocus
                    required
                    maxLength={256}
                    value={projectName}
                    onChange={(event) => setProjectName(event.target.value)}
                  />
                </label>
              )}
              <label>
                {projectAction === "open" ? "PASSPHRASE" : "NEW PASSPHRASE"}
                <input
                  autoFocus={projectAction !== "create"}
                  required
                  type="password"
                  autoComplete={
                    projectAction === "open"
                      ? "current-password"
                      : "new-password"
                  }
                  value={passphrase}
                  onChange={(event) => setPassphrase(event.target.value)}
                />
              </label>
              <p className="studio-form-hint">
                {projectAction === "open"
                  ? "The passphrase is kept in app memory only until you close the Project or quit Studio."
                  : projectAction === "restore"
                    ? "This decrypted JSON backup will be encrypted with your new passphrase. Keep the backup file protected."
                    : "Choose a strong new passphrase. It encrypts this Project and cannot be recovered if lost."}
              </p>
              <button
                type="submit"
                className="studio-button studio-primary"
                disabled={
                  busy || (projectAction === "restore" && !backupSummary)
                }
              >
                {busy
                  ? "WORKING..."
                  : projectAction === "create"
                    ? "CREATE & OPEN"
                    : projectAction === "restore"
                      ? "RESTORE & OPEN"
                      : "OPEN PROJECT"}
              </button>
            </form>
          </div>
        )}
      </main>
    )
  }

  const activeLobby = lobbies.find((lobby) => lobby.id === activeLobbyID)

  return (
    <main className="studio-root studio-workspace">
      <aside className="studio-sidebar">
        <header className="studio-brand">
          <img
            className="studio-brand-mark"
            src="/brand/rizzbase-mark-light.svg"
            alt=""
          />
          <img
            className="studio-brand-wordmark"
            src="/brand/rizzbase-wordmark-on-light.svg"
            alt="Rizzbase"
          />
          <span className="studio-product-label">STUDIO</span>
        </header>
        <div className="studio-project-chip">
          <span>OPEN PROJECT</span>
          <strong>{project.name}</strong>
          <small title={project.path}>{project.path}</small>
        </div>
        <div className="studio-sidebar-heading">
          <span>LOBBIES</span>
          <button
            type="button"
            aria-label="Create lobby"
            title="Create lobby"
            disabled={preview}
            onClick={() => {
              rememberDialogOpener()
              setError("")
              setShowLobbyForm(true)
            }}
          >
            +
          </button>
        </div>
        <nav className="studio-lobby-list" aria-label="Lobbies">
          {lobbies.map((lobby) => (
            <div
              className={`studio-lobby-item ${
                activeLobbyID === lobby.id ? "active" : ""
              }`}
              key={lobby.id}
            >
              <button
                type="button"
                className="studio-lobby-select"
                aria-pressed={activeLobbyID === lobby.id}
                onClick={() => setActiveLobbyID(lobby.id)}
              >
                <span className="studio-lobby-dot" aria-hidden="true" />
                <span className="studio-lobby-name">{lobby.name}</span>
                <small aria-hidden="true">{lobby.yapCount}</small>
                <span className="studio-sr-only">
                  {lobby.yapCount} {lobby.yapCount === 1 ? "Yap" : "Yaps"}
                </span>
              </button>
              <button
                type="button"
                className="studio-lobby-delete"
                aria-label={`Delete lobby ${lobby.name}`}
                title="Delete lobby"
                disabled={preview}
                onClick={() => void deleteLobby(lobby)}
              >
                ···
              </button>
            </div>
          ))}
          {lobbies.length === 0 && (
            <p className="studio-no-lobbies">
              No Lobbies yet. Create one to start adding data.
            </p>
          )}
        </nav>
        <footer className="studio-sidebar-footer">
          <button
            type="button"
            onClick={() => void exportBackup()}
            disabled={busy || preview}
          >
            EXPORT JSON BACKUP
          </button>
          <button
            type="button"
            onClick={() => void closeProject()}
            disabled={busy || preview}
          >
            CLOSE PROJECT
          </button>
          <div className="studio-contact">
            <a href="mailto:hello@rizzbase.tech">hello@rizzbase.tech</a>
            <a href="mailto:paul@rizzbase.tech">paul@rizzbase.tech</a>
          </div>
        </footer>
      </aside>

      <section className="studio-main">
        <header className="studio-topbar">
          <div>
            <span className="studio-eyebrow">PROJECT</span>
            <strong>{project.name}</strong>
          </div>
          <div className={`studio-local-badge ${preview ? "is-preview" : ""}`}>
            <i /> {preview ? "PREVIEW · SAMPLE DATA" : "LOCAL & ENCRYPTED"}
          </div>
        </header>
        {preview && (
          <div
            className="studio-inline-message studio-preview-message"
            role="status"
          >
            Sample data preview only. File operations are available in the Wails
            desktop app.
          </div>
        )}
        {error && (
          <div className="studio-inline-message studio-error" role="alert">
            {error}
            <button type="button" onClick={() => setError("")}>
              DISMISS
            </button>
          </div>
        )}
        {notice && (
          <div className="studio-inline-message studio-success" role="status">
            {notice}
            <button type="button" onClick={() => setNotice("")}>
              DISMISS
            </button>
          </div>
        )}
        {activeLobby ? (
          <div className="studio-content">
            <div className="studio-content-heading">
              <div>
                <span className="studio-eyebrow">
                  LOBBY · {yaps.length} YAPS
                </span>
                <h1>
                  {activeLobby.name}
                  <span>.</span>
                </h1>
                <p>Browse and edit JSON documents in this Lobby.</p>
              </div>
              <button
                type="button"
                className="studio-button studio-primary"
                disabled={busy || preview}
                onClick={startCreateYap}
              >
                + NEW YAP
              </button>
            </div>
            <div className="studio-table-wrap">
              <table className="studio-table">
                <thead>
                  <tr>
                    <th scope="col">ID</th>
                    <th scope="col">PREVIEW</th>
                    <th scope="col">ACTIONS</th>
                  </tr>
                </thead>
                <tbody>
                  {yaps.map((yap) => (
                    <tr key={yap.id}>
                      <td>
                        <code>{yap.id}</code>
                      </td>
                      <td>
                        <pre>{prettyJSON(yap.data)}</pre>
                      </td>
                      <td className="studio-row-actions">
                        <button
                          type="button"
                          aria-label={`Edit Yap ${yap.id}`}
                          disabled={preview}
                          onClick={() => startEditYap(yap)}
                        >
                          EDIT
                        </button>
                        <button
                          type="button"
                          aria-label={`Delete Yap ${yap.id}`}
                          disabled={preview}
                          onClick={() => void deleteYap(yap)}
                        >
                          DELETE
                        </button>
                      </td>
                    </tr>
                  ))}
                  {yaps.length === 0 && (
                    <tr>
                      <td className="studio-empty-row" colSpan={3}>
                        This Lobby is empty. Add a Yap to get started.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        ) : (
          <div className="studio-no-selection">
            <span className="studio-eyebrow">YOUR PROJECT IS READY</span>
            <h1>
              Create a Lobby
              <br />
              to get started.
            </h1>
            <button
              type="button"
              className="studio-button studio-primary"
              disabled={preview}
              onClick={() => {
                rememberDialogOpener()
                setError("")
                setShowLobbyForm(true)
              }}
            >
              + CREATE LOBBY
            </button>
          </div>
        )}
      </section>

      {(projectAction || showLobbyForm || isNewYap || editingYap) && (
        <div className="studio-modal-backdrop" role="presentation">
          {showLobbyForm ? (
            <form
              ref={dialogRef}
              className="studio-modal"
              role="dialog"
              aria-modal="true"
              aria-labelledby="studio-lobby-dialog-title"
              tabIndex={-1}
              onSubmit={(event) => void createLobby(event)}
            >
              <button
                className="studio-modal-close"
                type="button"
                aria-label="Close"
                disabled={busy}
                onClick={() => setShowLobbyForm(false)}
              >
                ×
              </button>
              <span className="studio-eyebrow">NEW LOBBY</span>
              <h2 id="studio-lobby-dialog-title">Create a Lobby.</h2>
              {error && (
                <div
                  className="studio-inline-message studio-error"
                  role="alert"
                >
                  {error}
                </div>
              )}
              <label>
                LOBBY NAME
                <input
                  autoFocus
                  required
                  maxLength={256}
                  value={newLobbyName}
                  onChange={(event) => setNewLobbyName(event.target.value)}
                />
              </label>
              <button
                type="submit"
                className="studio-button studio-primary"
                disabled={busy}
              >
                CREATE LOBBY
              </button>
            </form>
          ) : isNewYap || editingYap ? (
            <form
              ref={dialogRef}
              className="studio-modal studio-yap-modal"
              role="dialog"
              aria-modal="true"
              aria-labelledby="studio-yap-dialog-title"
              tabIndex={-1}
              onSubmit={(event) => void saveYap(event)}
            >
              <button
                className="studio-modal-close"
                type="button"
                aria-label="Close"
                disabled={busy}
                onClick={() => {
                  setIsNewYap(false)
                  setEditingYap(null)
                }}
              >
                ×
              </button>
              <span className="studio-eyebrow">
                {isNewYap ? "NEW DOCUMENT" : "EDIT DOCUMENT"}
              </span>
              <h2 id="studio-yap-dialog-title">
                {isNewYap ? "Add a Yap." : "Edit this Yap."}
              </h2>
              {error && (
                <div
                  className="studio-inline-message studio-error"
                  role="alert"
                >
                  {error}
                </div>
              )}
              <label>
                YAP ID
                <input
                  autoFocus
                  required
                  maxLength={128}
                  disabled={!isNewYap}
                  value={yapID}
                  onChange={(event) => setYapID(event.target.value)}
                />
              </label>
              <label>
                JSON DATA
                <textarea
                  className="studio-json-input"
                  required
                  spellCheck={false}
                  value={yapJSON}
                  onChange={(event) => setYapJSON(event.target.value)}
                />
              </label>
              <p className="studio-form-hint">
                Data must be a valid JSON value. Saving replaces the complete
                document.
              </p>
              <button
                type="submit"
                className="studio-button studio-primary"
                disabled={busy}
              >
                {busy ? "SAVING..." : "SAVE YAP"}
              </button>
            </form>
          ) : null}
        </div>
      )}
    </main>
  )
}
