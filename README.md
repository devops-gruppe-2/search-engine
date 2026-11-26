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
                                                                  ├─ Nginx      (port 80/443, HTTPS)
                                                                  ├─ Go-app     (port 8080, kun intern)
                                                                  ├─ PostgreSQL (ingen åbne porte)
                                                                  └─ Prometheus (sender til Grafana Cloud)
```

| Del | Teknologi |
|---|---|
| Backend | Go med Gin |
| Database | PostgreSQL, søgning med `tsvector` og GIN-index, migreringer med goose |
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

Go-testene kører mod en rigtig PostgreSQL. Hver test får sit eget schema, som slettes bagefter. Start databasen og kør testene:

```bash
docker compose up -d postgres
cd backend
TEST_DATABASE_URL="postgres://whoknows:whoknows@localhost:5432/whoknows?sslmode=disable" go test ./...
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
- **Login-krav:** sider, der kræver login, ligger i en route-gruppe med middlewaren `requireLogin` (`backend/main.go`).
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

Søgningen bruger PostgreSQL's fuldtekstsøgning i stedet for `LIKE`:
- Kolonnen `search_vector` (`tsvector`) udregnes af PostgreSQL, hver gang en side gemmes eller ændres. Et **GIN-index** gør, at søgningen ikke skal læse alle sider.
- Resultaterne sorteres efter relevans (`ts_rank`), og et ord i titlen tæller 10 gange mere end et ord i teksten.
- "languages" finder også "language", og småord som "the" og "is" ignoreres. Danske sider bruger den danske ordbog.

Tabellerne og indexet laves af migreringerne i `backend/migrations/` (goose), når appen starter.

Koden ligger i `backend/search.go`. Sammenlign selv hastigheden:

```bash
cd backend
TEST_DATABASE_URL="postgres://whoknows:whoknows@localhost:5432/whoknows?sslmode=disable" go test -bench Search -run '^$'
```

---

## 📊 Logning af søgninger

Hver søgning skrives som én linje JSON i `logs/searches.log` (med Go's `log/slog`). Brugernavn og IP-adresse logges ikke. Loggen viser, hvad brugerne søger efter, og især hvad de ikke finder, så vi ved, hvad crawleren skal hente.

Se en opsummering:

```bash
# På serveren
docker exec whoknows-backend ./searchstats

# Lokalt fra backend-mappen
go run ./cmd/searchstats -file logs/searches.log
```

---

## 📈 Monitoring (Prometheus og Grafana)

Appen tæller, hvordan brugerne bruger produktet, og viser tallene på `/metrics`. Prometheus henter dem hvert 15. sekund, og Grafana viser dem på dashboardet **"Who Knows – brugerne"**:

| Spørgsmål | Måling |
|---|---|
| Finder brugerne det, de søger efter? | `whoknows_searches_total{results="found"/"none"}` |
| Kan brugerne logge ind og oprette sig? | `whoknows_logins_total`, `whoknows_registrations_total` |
| Hvor mange brugere har vi, og hvor mange er aktive i dag? | `whoknows_users_total`, `whoknows_active_users_today` |
| Hvor længe venter brugerne? | `whoknows_request_duration_milliseconds` |

Kør det lokalt:

```bash
docker compose --profile monitoring up --build
```

- Appen: http://localhost:8080
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin / admin)

`/metrics` kan ikke ses udefra. Nginx svarer `404`, og Prometheus henter tallene direkte fra appen i Docker-netværket.

**På serveren** kører Prometheus i agent-mode (`monitoring/prometheus.agent.yml`). Den henter tallene fra appen og sender dem til **Grafana Cloud**, hvor dashboardet ligger. Så kører monitoring ikke på samme server som appen. API-tokenen ligger kun på serveren i `/opt/whoknows/secrets/grafana-cloud-token`.

Dashboardet kan ses uden login: https://dreamycaravan440.grafana.net/public-dashboards/37e50f266f314afa9bb21bf85f9115dc

---

## 🌦️ Vejr

`/weather` viser vejrudsigten for København de næste 7 dage, og `/api/weather` giver de samme data som JSON.

Data kommer fra [Open-Meteo](https://open-meteo.com/), som er gratis og ikke kræver en API-nøgle. Udsigten gemmes i 30 minutter, så vi ikke spørger Open-Meteo ved hvert besøg. Hvis Open-Meteo er nede, vises den senest hentede udsigt.

---

## 🕷️ Wikipedia-crawler

Søgemaskinen skal have noget at søge i. Crawleren henter Wikipedia-artikler fra en eller flere kategorier og gemmer dem i tabellen `pages`. Den venter mellem hvert kald, så Wikipedia ikke bliver belastet.

```bash
cd backend
DATABASE_URL="postgres://whoknows:whoknows@localhost:5432/whoknows?sslmode=disable" go run ./cmd/crawler -limit 200
```

| Flag | Standard | Betydning |
|---|---|---|
| `-db` | `DATABASE_URL` | Forbindelsen til PostgreSQL |
| `-lang` | `en` | Wikipedia-sprog (`en` eller `da`) |
| `-categories` | `Category:Programming languages,Category:Computer programming` | Kategorier adskilt med komma |
| `-limit` | `200` | Højst så mange artikler |
| `-delay` | `200ms` | Pause mellem kald |

På serveren ligger crawleren i app-containeren, som kender `DATABASE_URL`. Tag en backup først:

```bash
/opt/whoknows/backup-db.sh
docker exec whoknows-backend ./crawler -limit 800
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
| `.env` | `SESSION_KEY` og `POSTGRES_PASSWORD` (kun læsbar for `deploy`) |
| `app.db` | Den gamle SQLite-database. Bruges kun til den første import til PostgreSQL |
| `logs/searches.log` | Logning af søgninger |
| `db-backups/` | Daglige backups |
| `backup-db.sh` | Backup-scriptet (kopi af `scripts/backup-db.sh`) |
| `renew-cert.sh` | Fornyer HTTPS-certifikatet |

Nyttige kommandoer:

```bash
cd /opt/whoknows
docker compose -f compose.prod.yml ps
docker compose -f compose.prod.yml logs -f backend
docker exec -it whoknows-postgres psql -U whoknows -c "\dt"
sudo ufw status verbose
```

**Automatiske job (cron):**
- **02:00:** Backup af databasen med `pg_dump` til `db-backups/`. Backups ældre end 14 dage slettes. Loggen ligger i `backup.log`.
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
