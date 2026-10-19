# TaskBelay

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja.md) · [한국어](README_ko.md) · [Español](README_es.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · [Português](README_pt-BR.md)

**Technische Kontrolle für reale Entwicklungsaufgaben mit KI.**

TaskBelay ergänzt Codex, DeepSeek Harness, Claude Code und ZCode. Es hält das autorisierte Ziel, tatsächliche Codeänderungen und Prüfnachweise zusammen, damit Aufgaben über Sitzungen hinweg mit weniger wiederholten Entscheidungen fortgesetzt werden können. Code und technische Beurteilung bleiben Aufgabe des Agenten.

## Möglichkeiten

- Ziele, ausdrückliche Nichtziele sowie Schnittstellen-, Daten-, Abhängigkeits- und Entwurfsvorgaben während der Entwicklung erhalten. Änderungen innerhalb von Dateien prüfen, nicht nur Pfade. Abweichungen im bestehenden Auftrag korrigieren und den Nutzer nur bei geänderter Befugnis oder einer wichtigen Entscheidung fragen.
- Autorisierte lokale Prüfungen ausführen und tatsächliche Exit-Codes, Eingaben, Umgebungsbedingungen und Ergebnisse speichern. Behauptung, Ausführung und Abnahmeurteil bleiben getrennt. Relevante Änderungen lassen Nachweise veralten; unabhängige Prüfungen mit begründetem Eingabebereich können gültig bleiben. Neue Planung setzt kumulierte Kosten nicht zurück.
- Implementierung und Prüfung abwechseln, Anforderungen ändern und mit Ergebnissen, fehlgeschlagenen Versuchen und unklaren Ausführungen fortsetzen. Die jüngste Historie ist begrenzt; gültige Ergebnisse und kumulierte Kosten bleiben erhalten.

Befehlsprüfungen verwenden `taskbelay check run` über das native Prozesswerkzeug des Hosts mit dessen Berechtigungen und Umgebung. Die WebUI bietet Aufgabensuche, Host- und Statusfilter, Sprachauswahl, Benutzerentscheidungen und die Wiederherstellung von Ausführungen. Jedes Ziel benötigt eine erfolgreiche Abnahmeprüfung; akzeptierte bestehende Regressionen ersetzen sie nicht. Das Lesen gespeicherter Zustände schreibt keine Verbrauchsdaten.

Technische Fragen behalten bis zur Klärung ihre ursprüngliche Kennung; ausgelassener Text löscht sie nicht. Einschränkungen der Lieferung ergeben sich aus diesen gespeicherten Entscheidungen. Die Host-Nutzung unterscheidet fehlende, teilweise und vollständige Meldungen; nicht verfügbare Werte bleiben unbekannt. Die Methodenhilfe enthält Ein- und Ausgabebeispiele sowie Hinweise zur Fehlerbehandlung, ohne dass Quellcode nötig ist. Neue Task-Branches übernehmen keinen impliziten Upstream.

## Start

Node.js >=24 und ein installierter Coding-Host sind erforderlich. Entwicklung aus den Quellen benötigt außerdem Go >=1.26 und pnpm >=11 <12. Diese Quellen enthalten eine inkompatible Weiterentwicklung des technischen Vertrags. Veröffentlichte Pakete und stabile Supportnachweise sind unverändert. Für den neuen Vertrag müssen die Quellen gebaut werden. Alte Daten erhalten und ein separates Datenverzeichnis ausdrücklich wählen; es gibt keine Migration oder automatische Rücksetzung.

Öffentliche Installation:

```sh
npm install -g taskbelay@latest
taskbelay
```

Mit `taskbelay` öffnest du das Menü für Updates, die Installation in Agents, Prüfung und Reparatur, das Kontrollzentrum, das Desktop-Haustier, Installationsinformationen sowie Deinstallation und Datenlöschung. Mit den Pfeiltasten navigierst du, mit Enter bestätigst du, mit der Leertaste wählst du mehrere Einträge und mit A alle Einträge aus. Esc führt zurück und behält die Auswahl. Die Updateansicht zeigt die tatsächlichen aktuellen und gewünschten Versionen des Launchers und der installierten Plugins. Lokale Entwicklungsinstallationen behalten ihren Vertriebskanal.

Eine Deinstallation behält Daten. `delete-data` entfernt Aufgabenaufzeichnungen, Konfiguration und Haustiereinstellungen, behält aber Plugins und Registrierungen und erstellt leere Datenverzeichnisse an den ursprünglichen Pfaden. `delete-all` entfernt alle verwalteten Plugins und bestätigten Daten. Beide behalten den Launcher, die Desktop-Anwendung, den Projektcode und den Gesprächsverlauf des Agents. Gemeinsame Daten werden nach Verzeichnis bestätigt und standardmäßig an einen wiederherstellbaren Ort verschoben; endgültiges Löschen erfordert eine weitere Bestätigung. Kläre zuerst laufende oder ungewisse Ausführungen und bestätige ausdrücklich, dass alle ZCode-Fenster und -Sitzungen geschlossen sind. Siehe die [Befehlsreferenz](docs/COMMANDS_en.md). Diese Änderungen beschreiben den aktuellen Quellcode, keine neue Paketveröffentlichung.

Aktuelle Quellen lokal bauen und installieren:

```sh
pnpm install --frozen-lockfile --ignore-scripts
node scripts/taskbelay-local.mjs install --host codex --yes
```

Adapter nach der [Host-Anleitung](packages/codex/README.md) aktivieren und ein begrenztes Ergebnis anfordern:

```text
Verwende TaskBelay für eine Begrenzung fehlgeschlagener Anmeldungen. Erhalte die öffentliche Authentifizierungs-API und vorhandene Nutzerdaten; entwirf keine neue Kontoverwaltung. Führe relevante bestehende Tests aus und berichte tatsächliche Ergebnisse.
```

Beschreiben Sie das Ergebnis und die notwendigen Grenzen. Der Agent erklärt seinen Ansatz und arbeitet innerhalb Ihrer bestehenden Erlaubnis. Gewöhnliche Änderungen an Umsetzung und Prüfmethoden benötigen keine erneute Freigabe; geänderte Ziele oder ausdrückliche Einschränkungen erfordern Ihre tatsächliche Entscheidung. Gültige Prüfergebnisse werden wiederverwendet. Der Verbrauch wird laufend erfasst; Obergrenzen gelten nur, wenn Sie sie setzen. Die WebUI verbindet eine kompakte Aufgabenliste mit den Tabs Übersicht, Prüfprotokolle sowie Anforderungen und Verbrauch. Ausgaben werden direkt unter der jeweiligen Prüfung aufgeklappt; Antworten, Wiederherstellung und Archivierung bleiben verfügbar. Datensätze aktualisieren liest gespeicherte Inhalte; Aktuellen Code prüfen beobachtet die tatsächlichen Bedingungen erneut. Setzen Sie mit der gespeicherten Task ID im ursprünglichen Arbeitsbereich fort.

Der Agent zeigt Verzeichnis, Branch und vorhandene Änderungen und verwendet den von Ihnen gewählten oder aus dem Kontext eindeutigen Arbeitsbereich. Er fragt nur bei Unklarheiten, Konflikten oder notwendigen neuen Arbeitsbereichsoperationen. Das Anlegen von Branches und Worktrees sowie die Integration von Upstream-Änderungen sind optional und folgen der tatsächlichen Erlaubnis.

```sh
taskbelay webui start
```

## Grenzen

Strukturierte Hooks prüfen unterstützte Schreibpfade; Shell- und externe Änderungen werden an Prüfpunkten beobachtet. Nur von Core ausgeführte Prüfungen besitzen Core-Ausführungsnachweise. Deklarierte Eingaben und Bedingungen schließen ignored-Abhängigkeiten, Datenbanken und entfernte Dienste nicht automatisch ein. Semantische Begründungen sind sichtbar, aber kein Korrektheitsbeweis. TaskBelay ist weder Sandbox noch zusätzlicher Coding-Agent.

Core-Aufwand wird gemessen; Analyse, Wartezeit und Tokens des Hosts werden gemeldet und können unvollständig sein. Kleine mechanische Aufgaben sind mit dem nativen Agenten oft einfacher. Quelltests belegen Fähigkeiten, keine nachgewiesene Verringerung von Fehlern oder menschlicher Aufsicht. Abschluss erhält Dateien und erteilt keine Git-, Veröffentlichungs- oder Löschrechte.

## Weitere Informationen

[Product](docs/PRODUCT_en.md) · [Commands](docs/COMMANDS_en.md) · [Support and verification](docs/SUPPORT-MATRIX_en.md) · [Codex](packages/codex/README.md) · [DeepSeek](packages/deepseek/README.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md)
