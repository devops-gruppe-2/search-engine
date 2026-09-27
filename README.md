## 🛠️ Code Quality & Git Hooks (Linter)

We use **`prek`** (a high-performance, Rust-based git hook manager) combined with **`golangci-lint` (v2)** to ensure that our Go webserver is secure, optimized, and free of resource leaks (like unclosed HTTP bodies or missing contexts) before code gets committed.

Once activated, these checks will run automatically every time you execute `git commit`.

---

### 🚀 Setup Instructions for the Team

To get the linter running on your local machine, follow these 3 simple steps:

#### 1. Install `prek`
If you don't have `prek` installed on your machine yet, install it using your preferred package manager:

*   **macOS / Linux (Homebrew):**
    ```bash
    brew install j178/tap/prek
    ```
*   **Windows (Scoop):**
    ```bash
    scoop bucket add j178 https://github.com
    scoop install prek
    ```
*   **Alternative (Go toolchain):**
    ```bash
    go install ://github.com
    ```

#### 2. Enable the Git Hook
Navigate to the **root directory** of this repository and link `prek` to your local Git directory:

```bash
prek install
```
*This creates a lightweight script shim inside your local `.git/hooks/` directory.*

#### 3. Verify your Setup
Run the linter manually over all existing project files once to ensure your environment compiles successfully:

```bash
prek run --all-files
```
If everything is set up correctly, you will see a green **`golangci-lint...Passed`** status (or a list of issues you need to resolve in your local files before committing).

---

### 💡 Good to Know

*   **Zero Local Configuration Needed:** You don't need to manually install or update Go linters on your machine. `prek` reads our shared `prek.toml` and automatically downloads, caches, and runs the correct sandboxed binaries in the background.
*   **Targeted Scans:** The linter is configured to run specifically inside our `/backend` module where our `go.mod` lives, preventing path-resolution and type-checking glitches.
*   **Custom Rules:** You can see or adjust the specific active checksuites (such as `gosec` for SQL injection and `bodyclose` for HTTP leaks) inside the `backend/.golangci.yml` file.
