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
 
---

## ☁️ Server Access

The application is deployed on a DigitalOcean droplet running Ubuntu 26.04 LTS.

SSH:

```bash
ssh deploy@157.245.66.153
```

An authorized SSH key is required.

---

## 🐳 Docker on the server

The backend runs in a Docker container named:

```text
whoknows-backend
```

Check running containers:

```bash
docker ps
```

View logs:

```bash
docker logs whoknows-backend
```

Restart the backend:

```bash
docker restart whoknows-backend
```

The container uses:

```text
--restart unless-stopped
```

so it starts automatically after a VM reboot unless it has been manually stopped.

---

## 🚀 Automatic Deployment

The project uses GitHub Actions for automatic deployment.

When changes are merged to `main`:

1. A new Docker image is built
2. The image is pushed to GitHub Container Registry
3. GitHub Actions connects to the droplet using SSH
4. The VM pulls the newest image
5. The old container is replaced with the new version

Docker image:

```text
ghcr.io/devops-gruppe-2/search-engine:latest
```

Application:

```text
http://157.245.66.153
```

---

## 💾 Database

The application uses SQLite.

The active database on the droplet is:

```text
/opt/whoknows/app.db
```

The database is mounted into the Docker container as:

```text
/app/app.db
```

This keeps the database data when the container is replaced.

Open the database:

```bash
sqlite3 /opt/whoknows/app.db
```

Show tables:

```sql
.tables
```

Exit SQLite:

```sql
.quit
```

---

## 💾 Database Backup

Automatic database backup is configured on the droplet.

Backup script:

```text
/opt/whoknows/backup-db.sh
```

Local backups:

```text
/opt/whoknows/db-backups
```

Backups older than 14 days are deleted automatically.

The backup runs every day at 02:00 using cron.

Check the cron job:

```bash
crontab -l
```

Check the backup log:

```bash
cat /opt/whoknows/backup.log
```



---

## 🕷️ Wikipedia Crawler

The search engine needs content in the `pages` table. The crawler in `backend/cmd/crawler` fetches Wikipedia articles from one or more categories and saves them as pages (title, URL, language, last updated and the article as plain text).

It uses the Wikipedia API with a delay between requests and identifies itself with a User-Agent, so we don't put unnecessary load on Wikipedia. Running it again updates the articles that are already saved.

Run it locally from the `backend` folder:

```bash
go run ./cmd/crawler -db ./app.db -limit 200
```

Options:

| Flag | Default | Description |
|---|---|---|
| `-db` | `./app.db` | Path to the SQLite database |
| `-lang` | `en` | Wikipedia language (`en` or `da`) |
| `-categories` | `Category:Programming languages,Category:Computer programming` | Comma separated categories |
| `-limit` | `200` | Max number of articles |
| `-delay` | `200ms` | Pause between requests |

On the droplet the crawler is built as a Linux binary and placed in `/opt/whoknows/crawler`. Take a backup first, then run it as the `deploy` user:

```bash
/opt/whoknows/backup-db.sh
cd /opt/whoknows && ./crawler -db /opt/whoknows/app.db -limit 800
```

The output is logged to `/opt/whoknows/crawler.log` when it runs in the background.
