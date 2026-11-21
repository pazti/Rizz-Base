import { useState, type FormEvent } from "react"

type PackageEntry = {
  id: string
  architecture: string
  modified: string
}

const architectures = ["x86", "x64", "neutral", "arm", "arm64"]

export default function Packages() {
  const [packages, setPackages] = useState<PackageEntry[]>([])
  const [addingPackage, setAddingPackage] = useState(false)
  const [packageUrl, setPackageUrl] = useState("")
  const [architecture, setArchitecture] = useState("")

  function addPackage(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const url = new URL(packageUrl)
    const id = url.pathname.split("/").filter(Boolean).at(-1) || url.hostname
    setPackages((current) => [
      ...current,
      { id, architecture, modified: new Date().toLocaleDateString() },
    ])
    setPackageUrl("")
    setArchitecture("")
    setAddingPackage(false)
  }

  return (
    <main className="packages-screen">
      <p className="packages-intro">
        Specify one package URL for each architecture that your application
        supports. When a customer downloads your application, the Store will
        download the appropriate installer based on customer device
        architecture.
      </p>

      <button
        className="packages-add-button"
        type="button"
        onClick={() => setAddingPackage((current) => !current)}
      >
        Add package
      </button>

      {addingPackage && (
        <form className="packages-form" onSubmit={addPackage}>
          <section className="packages-form-section">
            <div className="packages-field-title">
              <label htmlFor="package-url">
                Package URL <span>*</span>
              </label>
              <a
                href="https://learn.microsoft.com/windows/apps/publish/"
                target="_blank"
                rel="noreferrer"
              >
                Learn more
              </a>
            </div>
            <p>
              Provide a versioned, secure URL for your standalone .exe or .msi
              installer package. To distribute an updated version of your
              application, you can update your versioned URL and submit it here.
            </p>
            <p className="packages-example">
              Example:
              https://www.contoso.com/contosoapp/downloads/1.1/setup.exe
            </p>
            <input
              id="package-url"
              autoFocus
              required
              type="url"
              value={packageUrl}
              onChange={(event) => setPackageUrl(event.target.value)}
              placeholder="Enter here"
            />
          </section>
          <section className="packages-form-section">
            <div className="packages-field-title">
              <span>
                Architecture<span className="packages-required">*</span>
              </span>
              <a
                href="https://learn.microsoft.com/windows/apps/publish/"
                target="_blank"
                rel="noreferrer"
              >
                Learn more
              </a>
            </div>
            <p>Select your application architecture.</p>
            <fieldset className="packages-architectures">
              <legend className="packages-visually-hidden">
                Select your application architecture
              </legend>
              {architectures.map((item) => (
                <label key={item}>
                  <input
                    required
                    type="radio"
                    name="architecture"
                    value={item}
                    checked={architecture === item}
                    onChange={(event) => setArchitecture(event.target.value)}
                  />
                  {item}
                </label>
              ))}
            </fieldset>
          </section>
          <div className="packages-form-actions">
            <button type="button" onClick={() => setAddingPackage(false)}>
              Cancel
            </button>
            <button type="submit">Add</button>
          </div>
        </form>
      )}

      <h1 className="packages-heading">List of packages</h1>

      <aside className="packages-validation" aria-label="Package validation">
        <span className="packages-info-icon" aria-hidden="true">
          i
        </span>
        <div>
          <strong>Package validation</strong>
          <p>
            We recommend you run validation checks on each of your app packages
            before you submit your app for review. Running validation checks
            early help you identify and fix errors.{" "}
            <a
              href="https://learn.microsoft.com/windows/apps/publish/"
              target="_blank"
              rel="noreferrer"
            >
              Learn more
            </a>
          </p>
          <p>
            <b>Note:</b> Each time you modify your package / silent parameters
            or code sign your package, you should re-run these validation
            checks.
          </p>
        </div>
      </aside>

      <div className="packages-table-wrap">
        <table className="packages-table">
          <thead>
            <tr>
              <th>Package ID</th>
              <th>Architecture</th>
              <th>Modified</th>
              <th>Package validation</th>
              <th>Validation status</th>
            </tr>
          </thead>
          <tbody>
            {packages.map((item, index) => (
              <tr key={`${item.id}-${index}`}>
                <td>{item.id}</td>
                <td>{item.architecture}</td>
                <td>{item.modified}</td>
                <td>Not run</td>
                <td>Not started</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </main>
  )
}
