# Ændringslog

Her dokumenterer vi alle ændringer i projektet: hvad der er ændret, hvorfor, og hvordan man kan se at det virker. Nyeste ændring står øverst.

Hver ændring i koden laves på en branch og merges til `main` via en pull request. Ændringer der ikke er kode (server, domæne, GitHub-indstillinger) står også her, så hele gruppen kan se hvad der er gjort.

---

## 2026-11-08 · Weather med Open-Meteo og cache (PR #55)

**Hvad**
- Ny side `GET /weather` og nyt endpoint `GET /api/weather` med vejrudsigten for København de næste 7 dage (vejr, max/min-temperatur og nedbør). `/api/weather` svarer med `{"data": {...}}`, som `StandardResponse` i OpenAPI-spec'en.
- Koden ligger i `backend/weather.go`. Backenden henter udsigten fra **Open-Meteo** og gemmer den i en **cache i 30 minutter**. En `sync.Mutex` sørger for, at kun én request ad gangen henter, så mange brugere på én gang ikke giver mange kald.
- Er Open-Meteo nede, vises den sidst gemte udsigt. Findes der ingen, viser siden en besked, og API'et svarer `503`.
- Link **Weather** (`#nav-weather`) i navigationen.
- **Tests:** 6 nye Go-tests (`backend/weather_test.go`) mod en **falsk Open-Meteo** i en `httptest`-server: udsigten vises rigtigt, cachen gør at Open-Meteo kun kaldes én gang, den gamle udsigt bruges når Open-Meteo fejler, fejlbesked uden udsigt, og oversættelsen af vejrkoder. 1 ny Playwright-test (`e2e/tests/weather.spec.js`) klikker på Weather-linket mod den rigtige Open-Meteo.
- **Dockerfile:** bygger nu hele pakken (`go build .`) i stedet for kun `main.go`, ellers kom `weather.go` ikke med. Rodcertifikaterne kopieres fra Go-imaget, fordi `debian:bookworm-slim` ikke har dem, og uden dem kan backenden ikke kalde Open-Meteo over HTTPS. CI's `build`-job bygger også hele pakken.
- **Fejl fundet af E2E-testene og rettet:** når to brugere registrerede sig samtidig, fik den ene `500`, fordi SQLite var låst af den anden ("database is locked"). Med 20 samtidige registreringer fejlede 9. Databasen åbnes nu med `busy_timeout(5000)`, så en request venter op til 5 sekunder på låsen i stedet for at fejle. Ny integrationstest `TestManyUsersCanRegisterAtTheSameTime` fejler uden rettelsen og består med den.

**Hvorfor**
- Opgaven *Consumer report* (uge 4): en weather-side med udsigten, integration med en 3. parts-tjeneste, og at integrationen testes i pipelinen. Simulatoren besøger også `/weather` og `/api/weather`.
- **Open-Meteo:** gratis til ikke-kommerciel brug, ingen API-nøgle (så ingen hemmelighed at gemme) og op til 10.000 kald om dagen.
- **Backend i stedet for frontend:** så virker cachen for alle brugere, og vi undgår browserens placering (geolocation), som kræver HTTPS og brugerens tilladelse.
- **Cache (skalering):** med 30 minutter kalder vi Open-Meteo højst ca. 48 gange i døgnet, uanset hvor mange brugere der er. Skal det skaleres til flere servere, kan cachen flyttes til en fælles cache som Redis.
- **Fast by:** søgemaskinen har ingen placering for brugeren, så vi viser København.

**Se at det virker**
- `cd backend && go test ./...` → `ok`
- https://whoknows-gruppe2.dk/weather viser en tabel med 7 dage, og https://whoknows-gruppe2.dk/api/weather giver JSON.
- GitHub → Actions → CI: `test` og `e2e` er grønne.

---

## 2026-11-07 · Integrationstests, unit tests og E2E-tests (PR #54)

**Hvad**
- **Integrationstests** (`backend/integration_test.go`, 12 tests): hver test starter hele appen med `setupRouter()` i en `httptest`-server med sin egen tomme SQLite-database og sender rigtige HTTP-requests med cookies. De tester:
  - register og login, forkert password, manglende felter, dublet-brugernavn
  - at passwords gemmes som bcrypt-hash
  - login i browseren, log ud-linket og logout
  - søgning, også på sprog, og søgning uden `q`
  - Security Breach: tvunget password-skift, blokering, de tre valideringsregler, og at det gamle password ikke virker bagefter
  - at forsiden og login-siden har de selectors, simulatoren bruger
- **Unit tests** (`backend/password_test.go`, 3 tests): `hashPassword` og `checkPassword` alene: rigtigt og forkert password, salt, og passwords over 72 bytes.
- **E2E-tests** med Playwright (`e2e/`, 5 tests), opsat som `07._E2E` i semester-repoet: en Chromium-browser søger, registrerer, logger ind og ud, og prøver et forkert password.
- **CI:** nyt job `e2e`, der starter appen med `docker compose`, kører Playwright og gemmer rapporten som artifact. `test`-jobbet kører `go test -v`. CD venter på hele CI, også `e2e`.
- **Branch protection:** `e2e` er tilføjet til de krævede checks.
- `main()` er delt op, så routes og middleware ligger i `setupRouter()`, som testene kan starte.
- **Fejl fundet af testene og rettet:** søgning gav `500`, hvis en side manglede `last_updated`, fordi `SELECT *` scannede `NULL` ind i en tekst. Søgningen nævner nu kolonnerne og bruger `COALESCE(last_updated, '')`.

**Hvorfor**
- Opgaven *Implement tests* (uge 9): integrationstests er et krav, E2E-tests tæller som integrationstests, og unit tests er valgfrie, men "nice to have a few".
- Eksamenskravet *You must perform testing*.

**Testtyper vi ikke har lavet**
- **Load/performance tests:** appen har få brugere, og simulatoren viser allerede, hvordan den klarer sig under belastning.
- **Mock-tests af databasen:** vi tester mod en rigtig SQLite-database i en temp-fil i stedet, fordi det er lige så hurtigt og tester de rigtige SQL-queries.
- **Tests af crawleren:** den kalder Wikipedia over nettet, og den kører kun manuelt.

**Se at det virker**
- `cd backend && go test ./...` → `ok`
- GitHub → Actions → CI: jobbene `test` og `e2e` er grønne, og `playwright-report` kan hentes som artifact.

---

## 2026-11-05 · Bcrypt og Security Breach (PR #53)

**Hvad**
- Passwords gemmes nu som **bcrypt-hash** i stedet for klartekst (`hashPassword` og `checkPassword` i `backend/main.go`). Det gælder både register, login og skift af password.
- Ny kolonne `must_change_password` i `users`.
- `migrateUsers` kører ved opstart: den tilføjer kolonnen, hasher alle passwords der stadig står i klartekst, og markerer de brugere som `must_change_password = 1`. Den kan køre flere gange uden at gøre skade, fordi den kun rører passwords, der ikke allerede er hashes.
- Ny side `/change-password` og nyt endpoint `POST /api/change-password`:
  - Siden fortæller brugeren om sikkerhedsbruddet.
  - Det nye password skal have mindst 8 tegn, skrives to gange ens og være forskelligt fra det gamle.
- Middleware `requirePasswordChange`: en bruger, der skal skifte password, bliver sendt til skifte-siden fra alle andre sider. API-kald får `403`. Kun skifte-siden, logout og CSS er tilladt.
- Login giver `403` med "Password must be changed" for API-kald og sender browseren til `/change-password`, når brugeren er markeret.

**Hvorfor**
- Opgaven *Security Breach* (uge 9): hackeren har adgang til databasen, og brugerne skal informeres og tvinges til at skifte password.
- Passwords i klartekst betyder, at alle kan læse dem, hvis databasen bliver stjålet. Bcrypt er langsom med vilje og bruger salt, så de ikke kan slås op i tabeller eller gættes hurtigt.
- Hele databasen må anses som kompromitteret, så **alle eksisterende brugere** skal skifte password, ikke kun de 20 fra listen på Teams. De brugere fra listen, vi har tjekket, findes ikke længere i databasen, fordi `DROP TABLE users` slettede brugere ved hvert deploy indtil PR #35.

**Se at det virker**
- Testet i Docker mod en gammel database med klartekst: migrering, login med 403 og redirect, blokering af andre sider, alle tre valideringsregler, skift af password, gammelt password afvist, nyt password virker, nye brugere får hash uden tvang, og genstart hasher ikke igen.
- `golangci-lint`: 0 issues.

**Bemærk**
- Første opstart på serveren hasher alle eksisterende brugere. Det tager ca. 1 minut, og siden svarer ikke imens.
- Backups fra før denne ændring i `/opt/whoknows/db-backups` indeholder stadig klartekst-passwords. De bliver slettet automatisk efter 14 dage.
- Brugere fra simulatoren bliver også tvunget til at skifte password og får `403` ved login, indtil de har gjort det.

---

## 2026-11-04 · fail2ban på serveren (server-ændring)

**Hvad**
- `fail2ban` er installeret på dropletten og slået til ved opstart (`systemctl enable --now fail2ban`).
- Ubuntus standardopsætning bruges: jail'en `sshd` læser SSH-loggen (systemd journal) og blokerer en IP med nftables i 10 minutter efter 5 mislykkede login-forsøg inden for 10 minutter.
- Login med password var allerede slået fra (`PasswordAuthentication no`), så man kun kan logge ind med SSH-nøgle.

**Hvorfor**
- Opgaven *fail2ban* (uge 9).
- Fra 25. okt. til 4. nov. var der over 5.500 mislykkede login-forsøg fra over 700 forskellige IP-adresser i `/var/log/auth.log`. Robotterne kan ikke komme ind uden nøgle, men de fylder loggen og bruger ressourcer.

**Se at det virker**
- `sudo fail2ban-client status sshd` viser antal mislykkede forsøg og hvilke IP'er der er blokeret. Den første IP blev blokeret få sekunder efter start.
- Vores egne logins og GitHub Actions bruger SSH-nøgle, så de fejler ikke og bliver ikke blokeret.

---

## 2026-11-04 · HTTPS med Nginx og Let's Encrypt (PR #50)

**Hvad**
- **Certifikat:** Hentet fra Let's Encrypt med certbot (Docker, `--standalone`) for `whoknows-gruppe2.dk`. Ligger på serveren i `/etc/letsencrypt/live/whoknows-gruppe2.dk/` og udløber 1. feb. 2027. Appen var stoppet i 12 sekunder, mens certifikatet blev lavet. Ingen mail er registreret hos Let's Encrypt.
- **`nginx/nginx.prod.conf`:** Nginx-konfiguration til serveren:
  - `http://whoknows-gruppe2.dk` sendes videre til `https://` (301).
  - Port 443 bruger certifikatet med TLS 1.2 og 1.3 og sender `Strict-Transport-Security`.
  - Kald direkte på IP-adressen (fx simulatoren via `repositories.py`) virker stadig over http, fordi certifikatet kun gælder for domænet.
  - `/.well-known/acme-challenge/` bruges til at forny certifikatet uden at stoppe Nginx.
- **`nginx/Dockerfile`:** eget Nginx-image med konfigurationen bygget ind, låst til `nginx:1.30.6-alpine`.
- **`compose.prod.yml`:** appen og Nginx kører sammen på serveren. Appen er kun tilgængelig for Nginx (`expose`), ikke direkte fra internettet.
- **`cd.yml`:** bygger også Nginx-imaget (tag `nginx-sha-<commit>` i samme pakke som appen), kopierer `compose.prod.yml` til serveren og starter begge med `docker compose`.
- `nginx/nginx.conf` og `compose.yml` er uændrede og bruges stadig lokalt og i CI.

**Hvorfor**
- Opgaven *Setup HTTPS* (uge 9), efter Anders' eksempel i `whoknows_variations` (branch `tls`).
- Uden HTTPS sendes passwords og session-cookies ukrypteret over nettet.

**Testet lokalt**
- Nginx-imaget og appen kørt sammen i Docker med et testcertifikat: `nginx -t` OK, IP over http giver 200, domæne over http giver 301 til https, https giver 200, `/api/search` virker, HSTS-header er sat, og acme-stien virker.
- Hadolint godkender `nginx/Dockerfile`.

**Efter merge**
- CD erstattede den gamle `docker run`-container med `whoknows-backend` og `whoknows-nginx` via compose. Port 8080 er ikke længere åben udefra.
- Tjekket på serveren: `https://whoknows-gruppe2.dk` giver 200 med gyldigt certifikat, `http://whoknows-gruppe2.dk` giver 301 til https, `http://157.245.66.153` giver stadig 200, og HSTS-headeren er sat.

**Fornyelse af certifikatet**
- `/opt/whoknows/renew-cert.sh` kører certbot med `--webroot` (Nginx kører videre) og genindlæser Nginx bagefter med `nginx -s reload`.
- Cron hos root kører den hver nat kl. 03:30 og logger til `/var/log/renew-cert.log`. Certbot fornyer kun, når der er under 30 dage tilbage.
- Certbot venter med vilje et tilfældigt antal minutter, når den kører automatisk. Test derfor med: `/opt/whoknows/renew-cert.sh --dry-run --no-random-sleep-on-renew` (bestod 2026-11-04).

**Opdateret andre steder**
- `repositories.py` hos underviseren: PR #44 i underviserens repo ændrer `backend` og `frontend` til `https://whoknows-gruppe2.dk/`.
- Postman-monitoren *WhoKnows Uptime* kalder nu `https://whoknows-gruppe2.dk/api/search?q=test`.

---

## 2026-11-03 · PR- og issue-templates (PR #49)

**Hvad**
- `.github/PULL_REQUEST_TEMPLATE.md`: hver ny PR starter med felterne *Hvad*, *Hvorfor* og *Hvordan er det testet*, og en tjekliste: grøn CI, ændringslog opdateret, ingen hemmeligheder i koden, endpoints følger OpenAPI.
- `.github/ISSUE_TEMPLATE/bug_report.yml`: formular til fejl med hvad der sker, hvordan man genskaber den, hvad der burde ske, hvor fejlen sker og log. Får labelen `bug`.
- `.github/ISSUE_TEMPLATE/feature_request.yml`: formular til nye funktioner og kursusopgaver med hvad, hvorfor, hvornår den er færdig, og link til opgaven. Får labelen `enhancement`.
- `.github/ISSUE_TEMPLATE/config.yml`: tomme issues er stadig tilladt.

**Hvorfor**
- Opgaverne *GitHub Pull Request template* (uge 3), *Create an issue template* (uge 4) og *Create an issue template* (uge 7).
- Alle PR'er og issues bliver skrevet på samme måde, så det er nemt at se, hvad en ændring gør, og hvornår en opgave er færdig. Tjeklisten minder om ændringsloggen.

**Se at det virker**
- *Pull requests → New pull request*: beskrivelsen er udfyldt med skabelonen.
- *Issues → New issue*: man kan vælge mellem *Fejl* og *Ny funktion eller opgave*.

---

## 2026-11-03 · CI/CD-oprydning (PR #48)

**Hvad**
- **CI er delt op i tre jobs:** `lint` (golangci-lint og Hadolint), `test` (`go test`) og `build` (build, Docker og smoke test). `build` kører kun, hvis `lint` og `test` er grønne.
- **CD venter på CI:** CD startede før samtidig med CI ved hvert push til `main`. Nu starter CD først, når CI er færdig på `main`, og kun hvis CI er grøn (`workflow_run`). CD kan også startes manuelt fra Actions-fanen (`workflow_dispatch`).
- **Image-tag med commit:** Docker-imaget får både `:latest` og `:sha-<commit>`. Serveren kører nu det præcise `sha`-image, som CI har testet.
- **Gamle triggers fjernet:** CI kørte også ved push til `feature/ci-cd` og `feature/smoke-testing`. Nu kun ved push til `main` og ved PR'er.
- **Branch protection:** de krævede checks er ændret fra `lint` og `test-build` til `lint`, `test` og `build`.

**Hvorfor**
- Separate jobs gør det tydeligt, hvad der fejler (lint, test eller build), og lint og test kører samtidig, så CI bliver hurtigere.
- Før kunne en commit på `main` blive deployet, selv om CI fejlede på den. Nu kommer kun kode, der har bestået CI, på serveren.
- Med `:latest` alene kan man ikke se, hvilken version der kører, og man kan ikke rulle tilbage. Med `sha`-tagget kan man se versionen med `docker ps` og deploye en ældre version igen.

**Se at det virker**
- GitHub → Actions → CI viser tre jobs: `lint` og `test` side om side, derefter `build`.
- CD starter først, når CI på `main` er grøn.
- På serveren: `docker ps` viser imaget `ghcr.io/devops-gruppe-2/search-engine:sha-<commit>`.

**Rulle tilbage**
```bash
IMAGE=ghcr.io/devops-gruppe-2/search-engine:sha-<gammel commit>
docker pull $IMAGE
docker stop whoknows-backend && docker rm whoknows-backend
docker run -d --name whoknows-backend --restart unless-stopped -p 80:8080 -v /opt/whoknows/app.db:/app/app.db $IMAGE
```

---

## 2026-11-03 · Branch protection på main (GitHub-indstilling, PR #47)

**Hvad**
- `main` er beskyttet under *Settings → Branches*:
  - Alle ændringer skal ind via en pull request. Man kan ikke pushe direkte til `main`.
  - CI-jobbene `lint` og `test-build` skal være grønne, før der kan merges.
  - Branchen skal være opdateret med `main`, før den merges (*strict*), så testene har kørt på den kode, der faktisk kommer på `main`.
  - Force push og sletning af `main` er ikke tilladt.
- Der kræves ingen godkendelse fra et andet gruppemedlem. Vi koordinerer reviews over telefon.
- Admins kan stadig gå udenom i en nødsituation.

**Hvorfor**
- Opgaven *Branch protection rules* (uge 4).
- Ændringer er blevet lagt direkte på `main` og merget uden at CI var kørt færdig. Nu kan der ikke komme kode på `main`, som fejler lint eller test.

**Se at det virker**
- En PR med rød CI kan ikke merges: knappen *Merge pull request* er grå.
- `git push origin main` bliver afvist.

---

## 2026-11-03 · Linter (golangci-lint) i CI (PR #46)

**Hvad**
- Nyt job `lint` i `ci.yml`, der kører golangci-lint v2.14.0 på `backend/` med reglerne i `backend/.golangci.yml`. Samme version som i `prek.toml`, så lokalt og CI giver samme resultat.
- `.golangci.yml` er rettet til v2-formatet: `disable-all` og `run.verbose` findes ikke i v2 og er erstattet med `default: standard`.
- Den eksisterende kode havde 13 fund, som er rettet:
  - **errcheck (6):** fejl fra `Close()` og `router.Run()` blev ignoreret. Nu bliver de logget, eller ignoreret med vilje (`_ =`).
  - **noctx (7):** databasekald brugte `Exec`, `Query` og `Ping` uden context. Nu bruges `...Context`-varianterne med requestens context, så et kald stoppes, hvis brugeren lukker forbindelsen.

**Hvorfor**
- Eksamenskrav: *Ensure software quality (static analysis / linting)*. Før kørte linteren kun lokalt for dem, der havde sat prek op.

**Se at det virker**
- GitHub → Actions → CI: jobbet *lint* er grønt. Fejler linteren, bliver PR'en rød.
- Lokalt: `cd backend && golangci-lint run ./...` giver `0 issues`.

---

## 2026-11-02 · Deploy uden tredjeparts-action og hash-pinning (PR #44)

**Hvad**
- `cd.yml` deployer nu med almindelig `ssh` i stedet for `appleboy/ssh-action`.
- SSH-nøglen skrives til en fil på GitHubs runner med `chmod 600` og slettes igen bagefter i et `if: always()`-step.
- `ssh-keyscan` gemmer serverens host key, så forbindelsen ikke stopper ved "are you sure?".
- Alle actions i `ci.yml` og `cd.yml` er låst til en commit-hash, fx `actions/checkout@fbc6f39... # v5`.

**Hvorfor**
- I uge 9 (*Security in GitHub*) blev `appleboy/ssh-action` brugt som eksempel på, at man ikke blindt skal give sine credentials til fremmed kode. Vi gav den vores private nøgle.
- Et versionstag som `@v5` kan flyttes til ny kode. En commit-hash kan ikke ændres, så vi ved præcis hvilken kode der kører i vores pipeline.

**Se at det virker**
- GitHub → Actions → CD: trinene *Deploy to DigitalOcean droplet* og *Remove SSH key* er grønne.

---

## 2026-11-02 · Dependabot slået til (GitHub-indstilling)

**Hvad**
- Dependabot alerts var allerede slået til. Dependabot security updates er slået til under *Settings → Advanced security*.

**Hvorfor**
- At holde pakker opdateret er den bedste måde at undgå kendte sikkerhedshuller (uge 9).

**Resultat**
- Dependabot fandt 30 alerts. Alle ligger i `legacy/src/backend/requirements.txt` (den gamle Python 2-app, som ikke kører nogen steder). Vores Go-backend havde ingen alerts.
- Dependabot oprettede selv PR #39-#42, som opdaterer de gamle Python-pakker. De kan lukkes, fordi legacy-koden ikke bruges.

---

## 2026-10-30 · Smoke test i CI (PR #38, Hawra)

**Hvad**
- `scripts/smoke-test.sh` henter forsiden og tjekker at den indeholder "¿Who Knows?".
- CI starter hele projektet med `docker compose up` og kører testen. Projektet stoppes igen bagefter.

**Hvorfor**
- Opgaven *Smoke testing* (uge 8): et hurtigt tjek af at systemet overhovedet starter og svarer.

---

## 2026-10-30 · Hadolint i CI (PR #37, Hawra)

**Hvad**
- Første trin i CI lint'er `backend/Dockerfile` med Hadolint.

**Hvorfor**
- Opgaven *Run hadolint locally and setup in CI pipeline* (uge 8). Hadolint finder dårlige vaner i Dockerfiles, fx images uden fast version.

**Mangler**
- Alle i gruppen skal også prøve at køre `hadolint backend/Dockerfile` lokalt.

---

## 2026-10-30 · Nginx reverse proxy i compose (Hawra)

**Hvad**
- `nginx/nginx.conf` og en Nginx-container i `compose.yml`, som sender trafik videre til Go-appen.

**Bemærk**
- Deployet til serveren bruger ikke `compose.yml`, så Nginx kører kun når man starter projektet med `docker compose up`. Det er forberedelse til HTTPS.

---

## 2026-10-29 · Domæne: whoknows-gruppe2.dk (one.com)

**Hvad**
- Domænet `whoknows-gruppe2.dk` er registreret på one.com med voucher fra Teams og aktiveret med MitID hos Punktum.dk.
- A-record for `whoknows-gruppe2.dk` peger på serveren `157.245.66.153`. Standard A/AAAA-records fra one.com er slået fra.

**Hvorfor**
- Forberedelse til *Setup HTTPS* (uge 9). Et certifikat kræver et domæne.

**Bemærk**
- `www` er ikke sat op. Det er valgfrit, og så skal `www` bare udelades i certbot-kommandoen.
- Gratis i 12 måneder. Abonnementet skal opsiges efter eksamen.

---

## 2026-10-29 · Wikipedia-crawler (PR #36)

**Hvad**
- `backend/cmd/crawler/main.go` er et separat Go-program, der henter artikler fra Wikipedia-kategorier via Wikipedias API og gemmer dem i tabellen `pages`.
- Søgeresultater viser nu kun de første 300 tegn af artiklen.
- Kørt på serveren: 419 artikler fra *Programming languages*, *Programming paradigms* og *Computer programming*, som blev til 417 unikke sider.

**Hvorfor**
- Søgningen gav ingen resultater, fordi `pages` var tom. Den oprindelige `whoknows.db` kunne ikke hentes, fordi serveren fra uge 1 er lukket.
- Første del af *Let them search* (uge 10): scrape og indeksér indhold.

**Hvordan**
- Bruger API'et i stedet for at scrape HTML, fordi API'et giver ren tekst og stabile felter.
- Sender en User-Agent og venter 200 ms mellem kald, så Wikipedia ikke belastes.
- `INSERT ... ON CONFLICT DO UPDATE`, så en ny kørsel opdaterer artikler i stedet for at lave dubletter.
- Se README-afsnittet *Wikipedia Crawler* for hvordan den køres.

---

## 2026-10-29 · Brugere slettes ikke længere ved deploy (PR #35)

**Hvad**
- `DROP TABLE IF EXISTS users` er fjernet fra `backend/schema.sqlite`.
- Admin-brugeren indsættes med `INSERT OR IGNORE`.

**Hvorfor**
- Filen køres hver gang appen starter, så alle brugere blev slettet ved hvert deploy.

---

## 2026-10-29 · Frontend følger kravene til id'er og legacy-layout (PR #34)

**Hvad**
- Fælles header og footer med `#nav-login`, `#nav-register` og `#nav-logout`.
- `#login-button` på `/login`. Login-formularen poster til `/api/login`.
- Login, register og logout sender browseren videre til en side i stedet for at vise JSON. API-kald uden `Accept: text/html` får stadig JSON.
- `GET /api/logout` som i OpenAPI-spec'en (`POST` virker stadig).
- `/` er søgesiden, og `style.css` bliver nu serveret med legacy-styling.

**Hvorfor**
- Simulatoren klikker rundt på siden og bruger de id'er, der står i *The Frontend Structure* i `commence_the_rewrite.md`.
- Feedback i uge 9: "Barely a website. No search results."

---

## 2026-10-29 · Deployment flyttet fra Azure til DigitalOcean (PR #33)

**Hvad**
- Appen kører på en DigitalOcean-droplet (`157.245.66.153`) i stedet for Keamandens Azure-VM.
- GitHub Actions logger ind som brugeren `deploy` via secrets `VM_HOST`, `VM_USER` og `VM_SSH_KEY`.
- Appen kører på port 80. Databasen ligger i `/opt/whoknows/app.db` og blev kopieret fra den nyeste backup på Azure.
- Daglig backup kl. 02:00 med cron i `/opt/whoknows/db-backups`. Backups ældre end 14 dage slettes.
- Postman-monitoren *WhoKnows Uptime* kalder `GET /api/search?q=test` hver time og sender mail ved fejl.
- `repositories.py` hos underviseren er opdateret med den nye IP (PR #41 i underviserens repo).

**Hvorfor**
- Keamanden forlod gruppen, og deployment må ikke afhænge af hans Azure-konto.

**Mangler**
- Backups ligger kun på samme server som databasen. De bør også kopieres et andet sted hen.

---

## Skabelon til nye ændringer

```markdown
## ÅÅÅÅ-MM-DD · Kort titel (PR #nr)

**Hvad**
- ...

**Hvorfor**
- ...

**Se at det virker**
- ...
```
