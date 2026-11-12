# ¿Who Knows? – søgemaskine

**Live:** https://whoknows-gruppe2.dk

¿Who Knows? er en lille søgemaskine. Man kan søge i artikler, oprette en bruger, logge ind og se vejrudsigten for København.

Projektet er lavet i faget DevOps på EK. Vi fik en gammel Python-app (`legacy/`), som var usikker og svær at drifte. Vi har skrevet den om til **Go** og bygget en DevOps-kæde rundt om den. Hver ændring bliver testet, sikkerhedsscannet og deployet automatisk.

Alle ændringer er beskrevet i **[ændringsloggen](docs/aendringslog.md)**. For hver ændring står der, hvad der er lavet, hvorfor, og hvordan man kan se, at det virker.

---

## 🧭 Sådan hænger det sammen

```text
 Udvikler                 GitHub                                   Server (DigitalOcean)
 ────────                 ──────                                   ─────────────────────
 git push  ──►  Pull request ──► CI: lint, test, e2e, build, trivy
                     │
                     ▼  (merge, når alt er grønt)
                   main  ──► CD: byg Docker-images ──► SSH ──►  docker compose
                                                                  ├─ Nginx  (port 80/443, HTTPS)
                                                                  └─ Go-app (port 8080, kun intern)
                                                                        └─ SQLite: app.db
```

| Del | Teknologi |
|---|---|
| Backend | Go med Gin |
| Database | SQLite, søgning med FTS5 |
| Frontend | HTML-templates og CSS fra backend |
| Webserver | Nginx (HTTPS og sikkerheds-headers) |
| Container | Docker og Docker Compose |
| CI/CD | GitHub Actions |
| Server | DigitalOcean-droplet med Ubuntu |
| Domæne og certifikat | `whoknows-gruppe2.dk` (one.com) og Let's Encrypt |

---

## 📁 Mapper

| Mappe | Indhold |
|---|---|
| `backend/` | Go-appen: `main.go` (routes, login, register), `search.go` (søgning), `weather.go` (vejr), templates og CSS |
| `backend/cmd/crawler/` | Program, der henter Wikipedia-artikler ind i databasen |
| `nginx/` | Nginx-opsætning til serveren og sikkerheds-headers |
| `e2e/` | Browser-tests med Playwright |
| `scripts/` | Smoke test |
| `.github/` | CI/CD-workflows, Dependabot og PR- og issue-skabeloner |
| `docs/` | Ændringsloggen |
| `legacy/` | Den gamle Python-app, kun til sammenligning. Den kører ikke. |

---

## ▶️ Kør projektet på din egen computer

Det kræver kun **Docker**:

```bash
docker compose up --build
```

Åbn http://localhost:8080. Stop igen med `Ctrl+C`.

Vil du køre Go-appen direkte uden Docker, skal du have **Go** installeret:

```bash
cd backend
go run .
```

---

## 🧪 Tests

| Type | Hvor | Hvad tester den |
|---|---|---|
| Unit tests | `backend/password_test.go` | Hashing af passwords (bcrypt) |
| Integrationstests | `backend/integration_test.go`, `search_test.go`, `weather_test.go`, `security_test.go` | Hele appen via HTTP med en rigtig database: register, login, logout, søgning, vejr og session-cookie |
| E2E-tests | `e2e/tests/` | En rigtig browser bruger siden som en bruger |
| Smoke test | `scripts/smoke-test.sh` | Appen starter i Docker, og forsiden svarer |

Alle tests kører automatisk i CI ved hver pull request.

Kør Go-testene (hver test får sin egen midlertidige database):

```bash
cd backend
go test ./...
```

Kør E2E-testene mod appen i Docker:

```bash
docker compose up --build -d
cd e2e
npm ci
npx playwright install chromium
npx playwright test
```

---

## 🔁 CI/CD

**CI** (`.github/workflows/ci.yml`) kører ved hver pull request. Alle fem jobs skal være grønne, før man kan merge (branch protection på `main`):

| Job | Hvad den gør |
|---|---|
| `lint` | golangci-lint på Go-koden og Hadolint på Dockerfilerne |
| `test` | Go-testene |
| `e2e` | Starter appen i Docker og kører Playwright-testene |
| `build` | Bygger appen og kører smoke testen |
| `trivy` | Scanner Docker-images for kendte sårbarheder (HIGH og CRITICAL) |

**CD** (`.github/workflows/cd.yml`) kører, når CI er grøn på `main`:
1. Bygger Docker-images til appen og Nginx og lægger dem på GitHub Container Registry med commit-nummeret som tag.
2. Forbinder til serveren med SSH og kopierer `compose.prod.yml` derover.
3. Starter de nye containere med `docker compose up -d`. Databasen ligger uden for containeren, så den bliver bevaret.
4. Tjekker siden med Mozilla Observatory og skriver karakteren i jobbets opsummering.

---

## 🔒 Sikkerhed

- **HTTPS** med Let's Encrypt. Http sendes videre til https, og HSTS er slået til.
- **Sikkerheds-headers** fra Nginx (`nginx/security-headers.conf`): CSP, X-Frame-Options, X-Content-Type-Options og Referrer-Policy. Mozilla Observatory giver **A+**.
- **Passwords** gemmes som bcrypt-hashes, aldrig i klartekst.
- **Session-cookien** er signeret med `SESSION_KEY` og er `Secure` og `HttpOnly`. Nøglen står i `/opt/whoknows/.env` på serveren og aldrig i Git.
- **SQL injection:** alle SQL-kald bruger parametre (`?`).
- **Trivy** stopper pull requests med kendte sårbarheder.
- **Dependabot** laver hver uge pull requests med opdateringer af Go-pakker, Docker-images, GitHub Actions og Playwright.
- **Serveren:** firewall (`ufw`) med kun port 22, 80 og 443 åbne, og `fail2ban` mod brute force på SSH.

Kør Trivy selv:

```bash
docker build -t whoknows-backend:scan ./backend
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy:0.69.3 image --severity HIGH,CRITICAL --ignore-unfixed whoknows-backend:scan
```

---

## 🔎 Søgning

Søgningen bruger SQLite **FTS5** (full text search) i stedet for `LIKE`:
- Resultaterne sorteres efter relevans, og et ord i titlen tæller mere end et ord i teksten.
- "languages" finder også "language".
- Den er ca. 47 gange hurtigere end `LIKE` på 5.000 sider.

Koden ligger i `backend/search.go`. Sammenlign selv hastigheden:

```bash
cd backend
go test -bench Search -run '^$'
```

---

## 🌦️ Vejr

`/weather` viser vejrudsigten for København de næste 7 dage, og `/api/weather` giver de samme data som JSON.

Data kommer fra [Open-Meteo](https://open-meteo.com/), som er gratis og ikke kræver en API-nøgle. Udsigten gemmes i 30 minutter, så vi ikke spørger Open-Meteo ved hvert besøg. Hvis Open-Meteo er nede, vises den senest hentede udsigt.

---

## 🕷️ Wikipedia-crawler

Søgemaskinen skal have noget at søge i. Crawleren henter Wikipedia-artikler fra en eller flere kategorier og gemmer dem i tabellen `pages`. Den venter mellem hvert kald, så Wikipedia ikke bliver belastet.

```bash
cd backend
go run ./cmd/crawler -db ./app.db -limit 200
```

| Flag | Standard | Betydning |
|---|---|---|
| `-db` | `./app.db` | Stien til databasen |
| `-lang` | `en` | Wikipedia-sprog (`en` eller `da`) |
| `-categories` | `Category:Programming languages,Category:Computer programming` | Kategorier adskilt med komma |
| `-limit` | `200` | Højst så mange artikler |
| `-delay` | `200ms` | Pause mellem kald |

På serveren ligger crawleren i `/opt/whoknows/crawler`. Tag en backup først:

```bash
/opt/whoknows/backup-db.sh
cd /opt/whoknows && ./crawler -db /opt/whoknows/app.db -limit 800
```

---

## ☁️ Serveren

Log ind (kræver en godkendt SSH-nøgle):

```bash
ssh deploy@157.245.66.153
```

Alt ligger i `/opt/whoknows`:

| Fil eller mappe | Hvad |
|---|---|
| `compose.prod.yml` | Kopieres hertil af CD og starter appen og Nginx |
| `.env` | `SESSION_KEY` (kun læsbar for `deploy`) |
| `app.db` | Databasen |
| `db-backups/` | Daglige backups |
| `backup-db.sh` | Backup-scriptet |
| `renew-cert.sh` | Fornyer HTTPS-certifikatet |

Nyttige kommandoer:

```bash
cd /opt/whoknows
docker compose -f compose.prod.yml ps
docker compose -f compose.prod.yml logs -f backend
sqlite3 app.db ".tables"
sudo ufw status verbose
```

**Automatiske job (cron):**
- **02:00:** Backup af databasen til `db-backups/`. Backups ældre end 14 dage slettes. Loggen ligger i `backup.log`.
- **03:30:** certbot tjekker certifikatet og fornyer det, når der er under 30 dage tilbage.

---

## 🛠️ Linter før commit (valgfrit)

Vi bruger [`prek`](https://github.com/j178/prek) sammen med `golangci-lint`, så Go-koden bliver tjekket, hver gang du laver `git commit`. Opsætningen står i `prek.toml` og `backend/.golangci.yml`. Den samme linter kører også i CI.

```bash
pip install prek
prek install
prek run --all-files
```

---

## 👥 Gruppe

DevOps-gruppe 2 på EK. Opgaver og fremdrift følges på [projekt-boardet](https://github.com/orgs/devops-gruppe-2/projects/1).
