# Workspaces and branches

[中文](WORKTREE-SOURCES.md) | [English](WORKTREE-SOURCES_en.md)

Default to the authorized current workspace/session; joining TaskBelay does not automatically create a branch or chat. Hosts prepare explicitly requested branches/worktrees. Retain authorization_ref, exact source/base, frozen commit, target branch, carry choice and operation outcome. Core verifies identity read-only and owns claims.

Optional local new_branch/current_branch and dedicated_worktree helpers retain source validation, initial content and uncertain-operation receipts. Reuse explicit choices and ask only for missing material inputs. Carry staged, unstaged and non-ignored content while preserving the source; ignored files are not automatically copied. scope returns current repositories, not a Core cursor.

Resume in the original bound instance. Branch switches, rewinds/rewrites and replacement instances cannot be blessed semantically; restore before continuing. Cross-workspace relocation is unsupported. Completion/cancellation retains directories, branches and code. Deletion needs separate real authorization, current terminal Core records and clean/pushed checks. Unknown, dirty or unpushed resources remain. See Host guides.
