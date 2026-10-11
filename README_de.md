# TaskBelay

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja.md) · [한국어](README_ko.md) · [Español](README_es.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · [Português](README_pt-BR.md)

**Technische Kontrolle für reale Entwicklungsaufgaben mit KI.**

TaskBelay ergänzt Codex, DeepSeek Harness, Claude Code und ZCode. Es hält das autorisierte Ziel, tatsächliche Codeänderungen und Prüfnachweise zusammen, damit Aufgaben über Sitzungen hinweg mit weniger wiederholten Entscheidungen fortgesetzt werden können. Code und technische Beurteilung bleiben Aufgabe des Agenten.

## Möglichkeiten

- Ziele, ausdrückliche Nichtziele sowie Schnittstellen-, Daten-, Abhängigkeits- und Entwurfsvorgaben während der Entwicklung erhalten. Änderungen innerhalb von Dateien prüfen, nicht nur Pfade. Abweichungen im bestehenden Auftrag korrigieren und den Nutzer nur bei geänderter Befugnis oder einer wichtigen Entscheidung fragen.
- Autorisierte lokale Prüfungen ausführen und tatsächliche Exit-Codes, Eingaben, Umgebungsbedingungen und Ergebnisse speichern. Behauptung, Ausführung und Abnahmeurteil bleiben getrennt. Relevante Änderungen lassen Nachweise veralten; unabhängige Prüfungen mit begründetem Eingabebereich können gültig bleiben. Neue Planung setzt kumulierte Kosten nicht zurück.
- Implementierung und Prüfung abwechseln, Anforderungen ändern und mit Ergebnissen, fehlgeschlagenen Versuchen und unklaren Ausführungen fortsetzen. Die jüngste Historie ist begrenzt; gültige Ergebnisse und kumulierte Kosten bleiben erhalten.

## Start

Node.js >=24 und ein installierter Coding-Host sind erforderlich. Entwicklung aus den Quellen benötigt außerdem Go >=1.26 und pnpm >=11 <12. Diese Quellen enthalten eine inkompatible Weiterentwicklung des technischen Vertrags. Veröffentlichte Pakete und stabile Supportnachweise sind unverändert. Für den neuen Vertrag müssen die Quellen gebaut werden. Alte Daten erhalten und ein separates Datenverzeichnis ausdrücklich wählen; es gibt keine Migration oder automatische Rücksetzung.

Öffentliche Installation:

```sh
npm install -g taskbelay@latest
taskbelay
```

Aktuelle Quellen lokal bauen und installieren:

```sh
pnpm install --frozen-lockfile --ignore-scripts
node scripts/taskbelay-local.mjs install --host codex --yes
```

Adapter nach der [Host-Anleitung](packages/codex/README.md) aktivieren und ein begrenztes Ergebnis anfordern:

```text
Verwende TaskBelay für eine Begrenzung fehlgeschlagener Anmeldungen. Erhalte die öffentliche Authentifizierungs-API und vorhandene Nutzerdaten; entwirf keine neue Kontoverwaltung. Führe relevante bestehende Tests aus und berichte tatsächliche Ergebnisse.
```

Ergebnis und notwendige Grenzen nennen. Bestehende Autorisierung bleibt gültig; interne Phasen und Verwaltungsformulare müssen nicht wiederholt bestätigt werden. Die lokale WebUI zeigt Ergebnisse, Lücken und notwendige Entscheidungen. Mit gespeicherter Task ID im ursprünglichen Arbeitsbereich fortsetzen.

```sh
taskbelay webui start
```

## Grenzen

Strukturierte Hooks prüfen unterstützte Schreibpfade; Shell- und externe Änderungen werden an Prüfpunkten beobachtet. Nur von Core ausgeführte Prüfungen besitzen Core-Ausführungsnachweise. Deklarierte Eingaben und Bedingungen schließen ignored-Abhängigkeiten, Datenbanken und entfernte Dienste nicht automatisch ein. Semantische Begründungen sind sichtbar, aber kein Korrektheitsbeweis. TaskBelay ist weder Sandbox noch zusätzlicher Coding-Agent.

Core-Aufwand wird gemessen; Analyse, Wartezeit und Tokens des Hosts werden gemeldet und können unvollständig sein. Kleine mechanische Aufgaben sind mit dem nativen Agenten oft einfacher. Quelltests belegen Fähigkeiten, keine nachgewiesene Verringerung von Fehlern oder menschlicher Aufsicht. Abschluss erhält Dateien und erteilt keine Git-, Veröffentlichungs- oder Löschrechte.

## Weitere Informationen

[Product](docs/PRODUCT_en.md) · [Commands](docs/COMMANDS_en.md) · [Support and verification](docs/SUPPORT-MATRIX_en.md) · [Codex](packages/codex/README.md) · [DeepSeek](packages/deepseek/README.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md)
