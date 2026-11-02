# Ændringslog

Her dokumenterer vi alle ændringer i projektet: hvad der er ændret, hvorfor, og hvordan man kan se at det virker. Nyeste ændring står øverst.

Hver ændring i koden laves på en branch og merges til `main` via en pull request. Ændringer der ikke er kode (server, domæne, GitHub-indstillinger) står også her, så hele gruppen kan se hvad der er gjort.

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
