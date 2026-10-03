<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="packages/webui/src/assets/taskbelay-wordmark-dark.svg" />
    <img src="packages/webui/src/assets/taskbelay-wordmark.svg" width="360" height="180" alt="TaskBelay" />
  </picture>
</p>

<h1 align="center">TaskBelay</h1>

<p align="center"><strong>Lange KI-Coding-Aufgaben, am sicheren Seil.</strong></p>

<p align="center">
  <a href="README.md">English</a> · <a href="README_zh-CN.md">简体中文</a> · <a href="README_zh-TW.md">繁體中文</a> · <a href="README_ja.md">日本語</a> · <a href="README_ko.md">한국어</a> · <a href="README_es.md">Español</a> · <a href="README_fr.md">Français</a> · <a href="README_de.md">Deutsch</a> · <a href="README_pt-BR.md">Português (Brasil)</a>
</p>

## Was du mit TaskBelay tun kannst

Dein Coding-Agent wählt den nächsten technischen Schritt.<br />
TaskBelay hält die Aufgabe unter Kontrolle.

Nutze es mit Codex, DeepSeek, Claude Code oder ZCode. Der Agent beurteilt den Code; TaskBelay bewahrt Umfang, Prüfgrenzen, Aufgabenstand und Wiederherstellungsdaten.

Beim Klettern wählt die kletternde Person die Route; die sichernde Person führt das Seil und fängt einen Sturz ab. TaskBelay überträgt diese Rollen auf die Programmierung: Der Agent trifft technische Entscheidungen, TaskBelay prüft den genehmigten Umfang und die Prüfgrenzen und nutzt gespeicherte Daten zur Wiederaufnahme nach Fehlern oder unklaren Ergebnissen. Es ist weder eine Sandbox noch ein weiterer Coding-Agent.

- **Klarer Umfang:** Gleiche tatsächliche Änderungen mit genehmigten Dateien ab. Arbeit außerhalb des Plans braucht eine Entscheidung.
- **Begrenzte Prüfung:** Plane relevante Prüfungen und ihre Grenzen. Weitere Prüfungen brauchen einen konkreten Grund.
- **Dauerhafter Zustand:** Der maßgebliche Aufgabenstand bleibt über Sitzungen hinweg lokal gespeichert.
- **Sichere Wiederherstellung:** Kläre Fehler oder ungewisse Ergebnisse anhand gespeicherter Zustände und Vorgänge, bevor du sie erneut ausführst.

Das eignet sich für Arbeiten an einem Repository, die mehrere Sitzungen dauern oder klare Grenzen
für Dateien und Tests benötigen. Für einzelne Fragen, Code-Erklärungen und kleine Änderungen ohne
gespeicherten Fortschritt ist die direkte Nutzung von Codex, DeepSeek, Claude Code oder ZCode meist einfacher.

## Schnellstart

> Installiere Node.js `>=24` und den gewünschten Host. Die nötigen Versionen und geprüften Plattformen stehen in der [Support-Matrix](docs/SUPPORT-MATRIX_en.md).

### 1. TaskBelay installieren

Installiere TaskBelay und folge anschließend den Anleitungen zur Aktivierung für [Codex](docs/CODEX_en.md), [DeepSeek](docs/DEEPSEEK_en.md), [Claude Code](docs/CLAUDE_en.md) oder [ZCode](docs/ZCODE_en.md). Das Paket unterstützt Windows x64 und macOS arm64; die native Prüfung von ZCode unter macOS steht noch aus.

```sh
npm install -g taskbelay@latest
taskbelay
```

Wähle deinen Host aus den Optionen des verwendeten Installers. Prüfe und bestätige in Codex den TaskBelay-Hook unter `/hooks`; starte in DeepSeek das gewählte Profile neu. Lade in Claude Code die Plugins neu oder beginne eine neue Unterhaltung und prüfe die angeforderten Berechtigungen.

Installiere und aktiviere das Plugin in ZCode unter Settings → Plugins und starte eine neue Unterhaltung, damit die Hooks wirksam werden. Ein lokal vorbereitetes Paket bestätigt noch nicht, dass ZCode es geladen hat.

### 2. Eine Aufgabe starten

Sende nach der jeweiligen Installation eine dieser Nachrichten in der Unterhaltung des Hosts:

**Codex**

```text
$taskbelay-codex:taskbelay Füge eine Rate-Limitierung für fehlgeschlagene Anmeldungen hinzu. Ändere nur Auth-Dateien und führe höchstens 4 gezielte Prüfungen aus.
```

**DeepSeek Harness**

```text
/taskbelay Füge eine Rate-Limitierung für fehlgeschlagene Anmeldungen hinzu. Ändere nur Auth-Dateien und führe höchstens 4 gezielte Prüfungen aus.
```

**Claude Code**

```text
/taskbelay-claude:taskbelay Füge eine Rate-Limitierung für fehlgeschlagene Anmeldungen hinzu. Ändere nur Auth-Dateien und führe höchstens 4 gezielte Prüfungen aus.
```

**ZCode**

Wähle im Eingabefeld unter `/` → Skills den Eintrag `taskbelay` und beschreibe die Aufgabe.

```text
Nutze TaskBelay, um fehlgeschlagene Anmeldungen zu begrenzen. Ändere nur Authentifizierungsdateien und führe höchstens 4 gezielte Prüfungen aus.
```

Sende diese Nachrichten im Gespräch, nicht im Terminal. Beschreibe Ziel, Abnahmekriterien,
Dateiumfang und Testgrenze.

Die erste Antwort bewertet die Anfrage und fragt, ob du direkt oder mit TaskBelay arbeiten möchtest.
Bei TaskBelay wird standardmäßig im aktuellen Verzeichnis ein neuer Aufgabenbranch vom aktuellen HEAD
erstellt. Bestätige den Branch und ob vorhandene, noch nicht committete Änderungen zur Aufgabe gehören.
Abhängigkeiten, lokale Konfiguration, Dateien und Index bleiben erhalten. Die Sitzung wird fortgesetzt,
wenn sie auf alle beteiligten Verzeichnisse zugreifen kann.

Du kannst ausdrücklich den aktuellen Branch weiterverwenden oder einen eigenen Git-Worktree erstellen.
Für einen Worktree wählst du zusätzlich die lokale oder entfernte Quelle und den Ausgangsbranch. Codex
öffnet das neue Verzeichnis, wenn der Host dies unterstützt; DeepSeek und Claude Code liefern den passenden Neustartbefehl.

Pro Verzeichnis ist nur eine aktive Task möglich. Auch manuelle Änderungen und Änderungen anderer
Werkzeuge werden erfasst; ein Branchwechsel während der Aufgabe hält den Ablauf an. Lokale Verzeichnisse
und Branches bleiben nach Abschluss erhalten. Noch nicht committete Änderungen müssen beim Start der
nächsten Aufgabe berücksichtigt werden.

Besprich vor der Implementierung die Anforderungen, den Entwurf, die Arbeitsschritte, die vorgesehenen Dateien und den Prüfplan. Die Entwicklung beginnt erst nach deiner ausdrücklichen Zustimmung zum gesamten Plan. Änderungen am Plan oder ein erweiterter Dateiumfang erfordern eine erneute Zustimmung. Die Wahl von TaskBelay oder eines Worktrees ersetzt diese Zustimmung nicht.

Um den Branchnamen einer aktiven Task zu korrigieren, bitte den Host, die Umbenennung in TaskBelay
vorzubereiten. Benenne danach nur den Branch eines Repositorys um; Verzeichnis, HEAD, Index und Dateien
bleiben gleich. TaskBelay prüft das Ergebnis und setzt dieselbe Task fort. Ein Abbruch ist nur möglich,
solange der ursprüngliche Zustand unverändert ist. In Codex lässt sich ein noch nicht ausgeführter Start
explizit ersetzen. Nach einem Fehler ist ein Nachweis nötig, dass kein Zielvorgang aufgerufen wurde.
Unklare Ergebnisse und teilweise ausgeführte Vorgänge müssen zuerst geprüft werden.

Auch nach vielen Planänderungen bleiben alle gespeicherten Baseline-Verweise in der Task erhalten, ohne
Archivstufe oder konfigurierte Höchstzahl. Verfügbare Ressourcen und der Wertebereich der Revisionsnummern
setzen weiterhin Grenzen. Antworten zeigen eine begrenzte erste Seite und die Gesamtzahl; bitte den Host,
ältere Entscheidungen über die vollständige, seitenweise Historie zu lesen. Schritte und Grenzen stehen
unter [Arbeitsbereichsvorgänge](docs/WORKTREE-SOURCES_en.md) und in der [Befehlsreferenz](docs/COMMANDS_en.md).

### 3. Fortsetzen und Fortschritt ansehen

Kehre nach einem Sitzungsneustart zum ursprünglichen Verzeichnis zurück und bitte darum, die Aufgabe
fortzusetzen. TaskBelay arbeitet ab dem gespeicherten Fortschritt weiter. Fehlt das Verzeichnis oder
wurde es ersetzt, pausiert die Aufgabe, bis du es wiederherstellst oder die Aufgabe ausdrücklich aufgibst.

Füge in DeepSeek Harness auch der Nachricht zum Fortsetzen `/taskbelay` hinzu.

Öffne in Claude das ursprüngliche Arbeitsverzeichnis und die ursprüngliche Unterhaltung und fordere mit `/taskbelay-claude:taskbelay` ausdrücklich die Fortsetzung der gespeicherten Aufgabe an.

Öffne in ZCode das ursprüngliche Arbeitsverzeichnis, wähle die TaskBelay Skill und bitte um Fortsetzung der gespeicherten Aufgabe. Wird ein neues Verzeichnis vorbereitet, folge den zurückgegebenen Anweisungen zum Öffnen des Arbeitsbereichs.

Diese Befehle verwenden den installierten globalen Manager. Bei einer Quellinstallation gilt der entsprechende Einstieg aus der Anleitung.

```bash
# Installierte Integrationen anzeigen
taskbelay status --host all

# Lokale Aufgabenansicht öffnen
taskbelay webui start
```

Nichtinteraktive Installation, eigene DSH Profiles, Updates, Reparatur und Entfernung beschreibt
die [Befehlsreferenz](docs/COMMANDS_en.md).

## Desktop-Maskottchen

Das Maskottchen benötigt einen eingerichteten Adapter und die installierte Desktop-Anwendung. Die Installation eines Adapters allein installiert die Desktop-Anwendung nicht.

Das Desktop-Maskottchen zeigt mehrere Aufgaben in gestapelten Sprechblasen und öffnet die jeweilige WebUI. Blockierte Aufgaben haben Vorrang. Nach Abschluss einer Aufgabe wechselt es automatisch zu einer noch offenen Aufgabe; du kannst auch eine Aufgabe anheften. Du kannst das Aussehen anpassen, Animationen steuern, die Größe ändern und das Maskottchen unabhängig starten oder stoppen.

```bash
taskbelay pet start
taskbelay pet stop
```

Die Desktop-Anwendungen sind für macOS arm64 und Windows 10/11 x64 vorgesehen. Installation und
Bedienung stehen im [Maskottchen-Handbuch](docs/DESKTOP-PETS_en.md), die verifizierte Verfügbarkeit
in der [Support-Matrix](docs/SUPPORT-MATRIX_en.md).

## Nutzungsgrenzen

TaskBelay steuert den Aufgabenablauf, nicht die Betriebssystemrechte. Es fängt nicht jeden Dateizugriff oder Shell-Befehl ab.

Ein eigener Worktree trennt Codeänderungen. Prozesse, Netzwerkzugriff, Zugangsdaten und externe
Dienste bleiben mit deiner Umgebung geteilt.

Der Abschluss einer Aufgabe erstellt nicht automatisch Commits, führt keinen Push aus und löscht
keinen Worktree. Diese Vorgänge erfordern deine gesonderte Zustimmung.

## Dokumentation

- **Nutzung:** [Codex](docs/CODEX_en.md) · [DeepSeek](docs/DEEPSEEK_en.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md) · [Befehle](docs/COMMANDS_en.md) · [Control Center](docs/WEBUI_en.md)
- **Projekt:** [Produktbeschreibung](docs/PRODUCT_en.md) · [Support-Matrix](docs/SUPPORT-MATRIX_en.md) · [Sicherheit](SECURITY.md)
- **Entwicklung und Beiträge:** [Dokumentationsübersicht](MANIFEST_en.md) · [Beitragsleitfaden](CONTRIBUTING.md)

## Community

[LINUX DO](https://linux.do/)

## Lizenz

[Apache License 2.0](LICENSE)
