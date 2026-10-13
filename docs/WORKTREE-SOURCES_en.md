# Workspaces and branches

[中文](WORKTREE-SOURCES.md) | [English](WORKTREE-SOURCES_en.md)

Use the workspace selected by the user or clear from context, showing directories, branches and existing changes. Ask only for ambiguity, conflicts or needed new workspace operations. Resume an existing Task in its bound directory. Hosts retain actual authorization_ref, source, branch, carry choice and operation results; Core observes identity and retains claims.

Optional local new_branch/current_branch and dedicated_worktree helpers retain source validation, initial content and uncertain-operation receipts. Reuse explicit choices and ask only for missing material inputs. Carry staged, unstaged and non-ignored content while preserving the source; ignored files are not automatically copied. scope returns current repositories, not a Core cursor.

Resume in the original bound instance. Branch switches, rewinds/rewrites and replacement instances cannot be blessed semantically; restore before continuing. Cross-workspace relocation is unsupported. Completion/cancellation retains directories, branches and code. Deletion needs separate real authorization, current terminal Core records and clean/pushed checks. Unknown, dirty or unpushed resources remain. See Host guides.
