<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="packages/webui/src/assets/taskbelay-wordmark-dark.svg" />
    <img src="packages/webui/src/assets/taskbelay-wordmark.svg" width="360" height="180" alt="TaskBelay" />
  </picture>
</p>

<h1 align="center">TaskBelay</h1>

<p align="center"><strong>오래 이어지는 AI 코딩, 빌레이의 보호와 함께.</strong></p>

<p align="center">
  <a href="README.md">English</a> · <a href="README_zh-CN.md">简体中文</a> · <a href="README_zh-TW.md">繁體中文</a> · <a href="README_ja.md">日本語</a> · <a href="README_ko.md">한국어</a> · <a href="README_es.md">Español</a> · <a href="README_fr.md">Français</a> · <a href="README_de.md">Deutsch</a> · <a href="README_pt-BR.md">Português (Brasil)</a>
</p>

## TaskBelay로 할 수 있는 일

기술적 판단은 Agent가 합니다.<br />
TaskBelay는 작업의 경계를 유지합니다.

Codex, DeepSeek, Claude Code 또는 ZCode와 함께 사용합니다. 코드를 이해하고 다음 기술적 행동을 선택하는 일은 Agent가 맡습니다.

등반에서 빌레이는 등반자가 경로를 선택하는 동안 확보자가 로프를 조절하고 추락을 막는 역할입니다. TaskBelay도 같은 방식으로 역할을 나눕니다. Agent가 기술적 판단을 내리고, TaskBelay는 승인된 범위와 검증 한도를 확인하며 실패하거나 결과가 불확실한 작업은 저장된 기록을 바탕으로 복구합니다. 샌드박스나 또 다른 코딩 Agent가 아닙니다.

- **명확한 범위:** 실제 변경을 승인된 파일과 비교합니다. 계획 밖의 작업에는 별도 결정이 필요합니다.
- **검증 한도:** 필요한 검사와 한도를 계획합니다. 검증을 늘리려면 구체적인 이유를 기록합니다.
- **지속되는 상태:** 세션이 바뀌어도 로컬에 저장된 작업 상태를 기준으로 이어갑니다.
- **안전한 복구:** 실패하거나 결과가 불확실하면 저장된 상태와 작업 기록을 확인한 뒤 재개나 재시도를 결정합니다.

여러 세션에 걸치거나 파일 범위와 테스트 작업량을 명확히 정해야 하는 저장소 작업에 적합합니다.
일회성 질문, 코드 설명, 진행 상황을 저장할 필요가 없는 작은 수정은 Codex, DeepSeek, Claude Code 또는 ZCode를 직접 쓰는 편이 간단합니다.

## 빠른 시작

> Node.js `>=24`와 사용할 Host를 먼저 설치하세요. 필요한 버전과 검증된 플랫폼은 [지원 표](docs/SUPPORT-MATRIX_en.md)를 확인하세요.

### 1. TaskBelay 설치

TaskBelay를 설치한 다음 [Codex](docs/CODEX_en.md), [DeepSeek](docs/DEEPSEEK_en.md), [Claude Code](docs/CLAUDE_en.md), [ZCode](docs/ZCODE_en.md) 안내에 따라 사용할 Host를 활성화하세요. 패키지는 Windows x64와 macOS arm64를 대상으로 하며, macOS ZCode의 실제 환경 검증은 아직 남아 있습니다.

```sh
npm install -g taskbelay@latest
taskbelay
```

사용하는 설치 메뉴에서 해당 Host를 선택하세요. Codex는 `/hooks`에서 TaskBelay hook을 검토하고 신뢰하며, DeepSeek는 선택한 Profile을 다시 시작합니다. Claude Code는 플러그인을 다시 불러오거나 새 대화를 열고 권한 안내를 확인하세요.

ZCode에서는 Settings → Plugins에서 플러그인을 설치하고 활성화한 뒤 새 대화를 시작해야 Hook이 적용됩니다. 로컬 패키지가 준비되었다고 해서 ZCode에서 로드되었다는 뜻은 아닙니다.

### 2. 작업 시작

해당 설치를 마친 뒤 Host 대화에 다음 메시지 중 하나를 보내세요.

**Codex**

```text
$taskbelay-codex:taskbelay 로그인 실패 속도 제한을 추가하세요. 인증 관련 파일만 변경하고 대상 확인을 최대 4개 실행하세요.
```

**DeepSeek Harness**

```text
/taskbelay 로그인 실패 속도 제한을 추가하세요. 인증 관련 파일만 변경하고 대상 확인을 최대 4개 실행하세요.
```

**Claude Code**

```text
/taskbelay-claude:taskbelay 로그인 실패 속도 제한을 추가하세요. 인증 관련 파일만 변경하고 대상 확인을 최대 4개 실행하세요.
```

**ZCode**

입력창의 `/` → Skills에서 `taskbelay`를 선택한 뒤 작업을 설명하세요.

```text
TaskBelay를 사용해 로그인 실패 속도 제한을 추가하세요. 인증 관련 파일만 변경하고 대상 확인을 최대 4개 실행하세요.
```

이 메시지는 터미널이 아닌 대화창에 입력합니다. 목표, 인수 조건, 파일 범위와 테스트 한도를 구체적으로 적으세요.

첫 응답은 요청을 평가하고 직접 개발할지 TaskBelay를 사용할지 묻습니다. TaskBelay를 선택하면 기본적으로
현재 디렉터리에서 현재 HEAD를 기준으로 새 작업 브랜치를 만듭니다. 새 브랜치와 기존 미커밋 변경을 작업에
포함할지 확인합니다. 의존성, 로컬 설정, 파일과 스테이징 상태를 유지하며, 모든 참여 디렉터리에 접근할 수
있으면 현재 세션에서 계속합니다.

현재 브랜치를 그대로 사용하거나 별도의 Git 작업 트리를 만들도록 명시적으로 선택할 수도 있습니다. 별도
작업 트리는 로컬 또는 원격 소스와 시작 브랜치도 선택합니다. Codex는 호스트가 지원하면 새 디렉터리를 열고,
DeepSeek와 Claude Code는 필요한 재시작 명령을 제공합니다.

한 디렉터리에는 활성 Task가 하나만 있을 수 있습니다. 수동 편집이나 다른 도구의 로컬 변경도 확인 대상이며,
작업 도중 브랜치를 바꾸면 진행이 일시 중지됩니다. 완료 후에도 로컬 디렉터리와 브랜치를 유지합니다.
다음 작업을 시작할 때는 남아 있는 미커밋 변경을 어떻게 처리할지 확인해야 합니다.

구현 전에 요구 사항, 설계, 작업 항목, 변경할 파일과 검증 계획을 확인하고 논의합니다. 전체 계획을 명시적으로 승인한 뒤 개발을 시작합니다. 계획을 수정하거나 파일 범위를 넓히면 다시 승인을 받습니다. TaskBelay나 워크트리 설정을 선택하는 것은 계획 승인과 별개입니다.

진행 중인 Task의 브랜치 이름을 고치려면 Host에게 TaskBelay에서 이름 변경을 먼저 준비하도록 요청한 뒤,
한 저장소의 브랜치 이름만 변경하세요. 디렉터리, HEAD, 인덱스와 파일은 유지해야 합니다. TaskBelay가 결과를
확인하면 같은 Task를 이어갑니다. 원래 상태가 바뀌기 전에만 이름 변경을 취소할 수 있습니다. Codex에서 아직
실행하지 않은 시작 요청은 명시적으로 대체할 수 있습니다. 실패한 요청에는 대상 작업을 호출하지 않았다는
증거가 필요하며, 결과가 불확실하거나 일부 실행된 작업은 먼저 확인해야 합니다.

계획을 여러 번 수정해도 저장된 기준선 참조 전체를 Task 안에 보관합니다. 별도 보관 계층이나 설정된 개수 상한은 없지만,
사용 가능한 자원과 수정 번호의 표현 범위에 따른 제한은 있습니다. 응답에는 크기가 제한된 첫 페이지와 총개수가 표시됩니다.
이전 결정을 검토할 때는 Host에게 전체 이력을 페이지별로 읽도록 요청하세요. 자세한 절차와 제한은
[작업 공간 작업](docs/WORKTREE-SOURCES_en.md)과 [명령 참조](docs/COMMANDS_en.md)를 확인하세요.

### 3. 재개 및 진행 상황 확인

세션을 다시 시작한 뒤 작업의 원래 작업 디렉터리로 돌아가 해당 작업을 계속해 달라고 명시적으로 요청하세요.
TaskBelay는 저장된 진행 상황부터 이어갑니다. 원래 작업 디렉터리가 없어지거나 교체되었다면 이를 복구하거나
작업을 명시적으로 포기할 때까지 일시 중지됩니다.

DeepSeek Harness에서는 재개를 요청하는 메시지에도 `/taskbelay`를 포함하세요.

Claude에서는 원래 작업 디렉터리와 대화로 돌아가 `/taskbelay-claude:taskbelay`로 저장된 작업을 계속한다고 명시하세요.

ZCode에서는 원래 작업 디렉터리를 열고 TaskBelay Skill을 선택한 뒤 저장된 작업을 계속해 달라고 요청하세요. 새 디렉터리를 준비했다면 반환된 작업 공간 열기 안내를 따르세요.

아래 명령은 설치된 전역 관리자를 사용합니다. 소스 설치에서는 안내서의 해당 실행 경로를 사용하세요.

```bash
# 설치된 연동 확인
taskbelay status --host all

# 로컬 작업 화면 열기
taskbelay webui start
```

비대화형 설치, 사용자 지정 DSH Profile, 업그레이드, 복구와 제거는
[명령어 참고서](docs/COMMANDS_en.md)를 확인하세요.

## 데스크톱 펫

펫을 사용하려면 설정된 Adapter와 설치된 데스크톱 앱이 모두 필요합니다. Adapter만 설치하면 데스크톱 앱은 설치되지 않습니다.

데스크톱 펫은 여러 작업을 겹친 말풍선으로 표시하고 각 작업의 WebUI를 엽니다. 차단된 작업을 우선 표시하며, 작업이 완료되면 다른 미완료 작업으로 자동 전환합니다. 특정 작업을 고정할 수도 있습니다. 외형 변경, 애니메이션 제어, 크기 조절, 개별 시작과 중지를 지원합니다.

```bash
taskbelay pet start
taskbelay pet stop
```

데스크톱 앱은 macOS arm64와 Windows 10/11 x64를 대상으로 합니다. 설치와 조작은
[펫 안내서](docs/DESKTOP-PETS_en.md), 검증된 사용 가능 범위는[지원 범위](docs/SUPPORT-MATRIX_en.md)를 참고하세요.

## 사용 제한

TaskBelay는 작업 흐름을 관리하며 OS 권한을 관리하지 않습니다. 모든 파일 작업이나 셸 명령을 가로채지는 않습니다.

전용 작업 트리는 코드 변경 사항을 분리합니다. 프로세스, 네트워크, 자격 증명과 외부 서비스는 현재 환경과 공유됩니다.

작업이 완료되어도 코드 커밋, 푸시 또는 작업 트리 삭제는 자동으로 실행되지 않습니다. 이 작업에는 별도의 승인이 필요합니다.

## 문서

- **사용 방법:** [Codex](docs/CODEX_en.md) · [DeepSeek](docs/DEEPSEEK_en.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md) · [명령어](docs/COMMANDS_en.md) · [Control Center](docs/WEBUI_en.md)
- **프로젝트:** [제품 정의](docs/PRODUCT_en.md) · [지원 범위](docs/SUPPORT-MATRIX_en.md) · [보안](SECURITY.md)
- **개발 및 기여:** [문서 목록](MANIFEST_en.md) · [기여 안내](CONTRIBUTING.md)

## 커뮤니티

[LINUX DO](https://linux.do/)

## 라이선스

[Apache License 2.0](LICENSE)
