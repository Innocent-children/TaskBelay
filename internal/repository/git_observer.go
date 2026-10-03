package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

const gitExecutable = "git"

type gitReadCommand uint8

const (
	gitShowWorktreeRoot gitReadCommand = iota + 1
	gitShowCommonDirectory
	gitShowWorktreeGitDirectory
	gitShowBranch
	gitShowHead
	gitShowHeadTree
	gitShowStatus
	gitShowRemoteBase
	gitShowLocalBase
	gitShowTaskSurface
	gitShowWorktreeDelta
	gitShowBaseEntries
	gitShowIndexEntries
	gitIsAncestor
	gitShowMergeCommits
	gitShowWholeIndex
	gitShowBranchRef
)

type GitObserver struct{ runner gitCommandRunner }

func NewGitObserver() *GitObserver {
	return &GitObserver{runner: gitCommandRunner{timeout: domain.GitCommandTimeout, outputLimit: int64(domain.MaxGitCommandOutputBytes)}}
}

var _ WorkspaceRepositoryObserver = (*GitObserver)(nil)

func (o *GitObserver) IdentifyWorkspace(ctx context.Context, repositoryPath string) (string, domain.Digest, error) {
	if err := validateRepositoryPath(repositoryPath); err != nil {
		return "", "", err
	}
	if o == nil {
		return "", "", ErrGitObservation
	}
	rootResult, err := o.required(ctx, gitShowWorktreeRoot, repositoryPath, "")
	if err != nil {
		return "", "", err
	}
	root, err := canonicalGitDirectory(rootResult.stdout)
	if err != nil {
		return "", "", err
	}
	commonResult, err := o.required(ctx, gitShowCommonDirectory, root, "")
	if err != nil {
		return "", "", err
	}
	common, err := canonicalGitDirectory(commonResult.stdout)
	if err != nil {
		return "", "", err
	}
	commonIdentity, err := gitDirectoryIdentity(common)
	if err != nil {
		return "", "", err
	}
	gitDirResult, err := o.required(ctx, gitShowWorktreeGitDirectory, root, "")
	if err != nil {
		return "", "", err
	}
	gitDir, err := canonicalGitDirectory(gitDirResult.stdout)
	if err != nil {
		return "", "", err
	}
	fileIdentity, err := gitDirectoryIdentity(gitDir)
	if err != nil {
		return "", "", err
	}
	return root, digestWorktreeInstance(root, common, commonIdentity, gitDir, fileIdentity), nil
}

// Observe identifies the current worktree instance without assuming a Task
// base. Application uses the stable instance digest to find an existing claim.
func (o *GitObserver) Observe(ctx context.Context, repositoryPath string) (domain.RepositoryBinding, error) {
	origin, binding, err := o.observe(ctx, repositoryPath, WorkspaceOriginSelection{}, nil, false)
	_ = origin
	return binding, err
}

func (o *GitObserver) ObserveWorkspace(ctx context.Context, repositoryPath string, selection WorkspaceOriginSelection, previous *domain.RepositoryBinding) (domain.WorkspaceOrigin, domain.RepositoryBinding, error) {
	return o.observe(ctx, repositoryPath, selection, previous, true)
}

func (o *GitObserver) observe(ctx context.Context, repositoryPath string, selection WorkspaceOriginSelection, previous *domain.RepositoryBinding, provisioned bool) (domain.WorkspaceOrigin, domain.RepositoryBinding, error) {
	if err := validateRepositoryPath(repositoryPath); err != nil || o == nil {
		if err != nil {
			return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
		}
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, ErrGitObservation
	}
	rootResult, err := o.required(ctx, gitShowWorktreeRoot, repositoryPath, "")
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	canonicalRoot, err := canonicalGitDirectory(rootResult.stdout)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	commonResult, err := o.required(ctx, gitShowCommonDirectory, canonicalRoot, "")
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	commonDir, err := canonicalGitDirectory(commonResult.stdout)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	commonDirectoryIdentity, err := gitDirectoryIdentity(commonDir)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	gitDirResult, err := o.required(ctx, gitShowWorktreeGitDirectory, canonicalRoot, "")
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	gitDir, err := canonicalGitDirectory(gitDirResult.stdout)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	worktreeGitDirectoryIdentity, err := gitDirectoryIdentity(gitDir)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	branchResult, err := o.runner.run(ctx, gitShowBranch, canonicalRoot, "")
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	branch, detached, err := observedBranch(branchResult)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	headResult, err := o.required(ctx, gitShowHead, canonicalRoot, "")
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	head, err := parseObjectLine(headResult.stdout)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	treeResult, err := o.required(ctx, gitShowHeadTree, canonicalRoot, "")
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	headTree, err := parseObjectLine(treeResult.stdout)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}

	baseCommit := head
	taskBranch := ""
	if branch != nil {
		taskBranch = *branch
	}
	if provisioned {
		if !validOriginSelection(selection) {
			return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, ErrProvisioningRequired
		}
		baseCommit, taskBranch = selection.BaseCommit, selection.TaskBranch
		if previous == nil {
			operation, value := gitShowLocalBase, selection.BaseBranch
			if selection.SourceType == "remote" {
				operation, value = gitShowRemoteBase, selection.RemoteName+"\x00"+selection.BaseBranch
			}
			remote, runErr := o.runner.run(ctx, operation, canonicalRoot, value)
			if runErr != nil || remote.exitCode != 0 {
				return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, ErrProvisioningRequired
			}
			remoteCommit, parseErr := parseObjectLine(remote.stdout)
			if parseErr != nil || remoteCommit != selection.BaseCommit {
				return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, ErrProvisioningRequired
			}
		}
	}
	statusResult, err := o.required(ctx, gitShowStatus, canonicalRoot, "")
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	statusRecords, err := parsePorcelainV2(statusResult.stdout)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, ErrGitObservation
	}
	if err := ensureNoDirtySubmodules(statusRecords); err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	pathStates, err := prepareFingerprintPaths(canonicalRoot, statusRecords)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	surfaceResult, err := o.required(ctx, gitShowTaskSurface, canonicalRoot, baseCommit)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	surfacePaths, surfaceKinds, err := parseNameStatus(surfaceResult.stdout)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	worktreeResult, err := o.required(ctx, gitShowWorktreeDelta, canonicalRoot, "")
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	worktreePaths, worktreeKinds, err := parseNameStatus(worktreeResult.stdout)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	for _, path := range worktreePaths {
		if surfaceKinds[path] == "" {
			surfacePaths = append(surfacePaths, path)
		}
		surfaceKinds[path] = mergeChangeKind(surfaceKinds[path], worktreeKinds[path])
	}
	for _, record := range statusRecords {
		if record.kind == "?" && surfaceKinds[record.path] == "" {
			surfacePaths = append(surfacePaths, record.path)
			surfaceKinds[record.path] = "A"
		}
	}
	changedPaths := observedChangedPaths(statusRecords)
	changedKinds := make(map[string]string, len(changedPaths))
	for _, record := range statusRecords {
		changedKinds[record.path] = recordStatusKind(record)
	}
	layers, err := o.readEntryLayers(ctx, canonicalRoot, baseCommit, worktreeKinds, changedPaths, surfacePaths)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	changedEntries, err := o.entries(ctx, canonicalRoot, baseCommit, changedPaths, changedKinds, layers)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	taskSurface, err := o.entries(ctx, canonicalRoot, baseCommit, surfacePaths, surfaceKinds, layers)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}

	instance := digestWorktreeInstance(canonicalRoot, commonDir, commonDirectoryIdentity, gitDir, worktreeGitDirectoryIdentity)
	origin := domain.WorkspaceOrigin{
		Mode: selection.Mode, SourceType: selection.SourceType, CarryChanges: selection.CarryChanges, RemoteName: selection.RemoteName, BaseBranch: selection.BaseBranch,
		BaseCommit: baseCommit, TaskBranch: taskBranch, SourceRepositoryGroupDigest: digestGitDirectory(commonDir, commonDirectoryIdentity, commonDirectoryDigestDomain),
		CanonicalWorktreeRoot: canonicalRoot, WorktreeGitDirDigest: digestWorktreeGitDirectory(gitDir, worktreeGitDirectoryIdentity),
		ProvisioningReceiptID: selection.ProvisioningReceiptID,
	}
	if !provisioned {
		origin.Mode = domain.WorkspaceModeDedicatedWorktree
		origin.SourceType = "remote"
		origin.RemoteName, origin.BaseBranch, origin.ProvisioningReceiptID = "observed", taskBranch, "observed-worktree"
	}
	identity := digestWorkspaceIdentity(origin, instance)
	relation, history, err := o.history(ctx, canonicalRoot, origin, instance, branch, detached, head, previous)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	content := digestWorkspaceContent(baseCommit, taskSurface)
	baseAncestorResult, err := o.runner.run(ctx, gitIsAncestor, canonicalRoot, baseCommit+"\x00"+head)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	baseAncestor := baseAncestorResult.exitCode == 0
	binding := domain.RepositoryBinding{
		WorktreeInstanceDigest: instance, IdentityDigest: identity, HistoryDigest: history, ContentDigest: content,
		CurrentBranch: branch, Detached: detached, CurrentHead: head, HeadTree: headTree, HistoryRelation: relation,
		BaseCommitAncestor: baseAncestor,
		ChangedEntries:     changedEntries, TaskSurface: taskSurface, ObservedAt: time.Now().UTC(),
	}
	binding.BindingDigest = digestRepositoryBinding(binding)
	if provisioned {
		commonDigest := digestGitDirectory(commonDir, commonDirectoryIdentity, commonDirectoryDigestDomain)
		gitDigest := digestWorktreeGitDirectory(gitDir, worktreeGitDirectoryIdentity)
		if origin.Validate() != nil || selection.Mode == domain.WorkspaceModeDedicatedWorktree && gitDir == commonDir || commonDigest != origin.SourceRepositoryGroupDigest || gitDigest != origin.WorktreeGitDirDigest {
			return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, ErrProvisioningRequired
		}
		if previous == nil && (detached || branch == nil || *branch != selection.TaskBranch || head != selection.BaseCommit || !selection.CarryChanges && (len(changedEntries) != 0 || len(taskSurface) != 0)) {
			return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, ErrProvisioningRequired
		}
	}
	if binding.Validate() != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, ErrGitObservation
	}
	if err := o.verifyStable(ctx, canonicalRoot, commonResult.stdout, commonDirectoryIdentity, gitDirResult.stdout, worktreeGitDirectoryIdentity, branchResult, headResult.stdout, statusRecords); err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	if err := verifyFingerprintPaths(canonicalRoot, pathStates); err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, err
	}
	return origin, binding, nil
}

func ValidWorkspaceOriginSelection(s WorkspaceOriginSelection) bool {
	return len(WorkspaceOriginViolations("workspace_origin", s)) == 0
}

func validOriginSelection(s WorkspaceOriginSelection) bool {
	return ValidWorkspaceOriginSelection(s)
}

func validRemoteRefName(value string) bool {
	if value == "" || len(value) > domain.MaxIdentifierBytes || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func validBranchRefName(value string) bool {
	if value == "" || len(value) > domain.MaxRepositoryPathBytes || strings.TrimSpace(value) != value || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".") || strings.Contains(value, "..") || strings.Contains(value, "@{") || strings.ContainsAny(value, " ~^:?*[\\") || value == "@" {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return false
		}
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func (o *GitObserver) required(ctx context.Context, command gitReadCommand, root, value string) (gitCommandResult, error) {
	result, err := o.runner.run(ctx, command, root, value)
	if err != nil {
		return gitCommandResult{}, err
	}
	if result.exitCode != 0 {
		if command == gitShowWorktreeRoot {
			return gitCommandResult{}, ErrNotGitRepository
		}
		return gitCommandResult{}, ErrGitObservation
	}
	return result, nil
}

func (o *GitObserver) history(ctx context.Context, root string, origin domain.WorkspaceOrigin, instance domain.Digest, branch *string, detached bool, head string, previous *domain.RepositoryBinding) (domain.RepositoryHistoryRelation, domain.Digest, error) {
	relation := domain.RepositoryHistoryExact
	priorHead := origin.BaseCommit
	priorDigest := domain.Digest("")
	expectedBranch := origin.TaskBranch
	if previous != nil {
		priorHead, priorDigest = previous.CurrentHead, previous.HistoryDigest
		if previous.CurrentBranch != nil && !previous.Detached {
			expectedBranch = *previous.CurrentBranch
		}
		_ = instance
	}
	if detached {
		relation = domain.RepositoryHistoryDetached
	} else if branch == nil || *branch != expectedBranch {
		relation = domain.RepositoryHistoryBranchChanged
	} else if head == priorHead {
		if previous != nil {
			return domain.RepositoryHistoryExact, previous.HistoryDigest, nil
		}
	} else {
		forward, err := o.runner.run(ctx, gitIsAncestor, root, priorHead+"\x00"+head)
		if err != nil {
			return "", "", err
		}
		if forward.exitCode == 0 {
			merges, mergeErr := o.runner.run(ctx, gitShowMergeCommits, root, priorHead+"\x00"+head)
			if mergeErr != nil {
				return "", "", mergeErr
			}
			if merges.exitCode == 0 && len(bytes.TrimSpace(merges.stdout)) == 0 {
				relation = domain.RepositoryHistoryLinearAdvance
			} else {
				relation = domain.RepositoryHistoryRewrite
			}
		} else {
			reverse, reverseErr := o.runner.run(ctx, gitIsAncestor, root, head+"\x00"+priorHead)
			if reverseErr != nil {
				return "", "", reverseErr
			}
			if reverse.exitCode == 0 {
				relation = domain.RepositoryHistoryRewind
			} else {
				relation = domain.RepositoryHistoryRewrite
			}
		}
	}
	return relation, digestHistory(priorDigest, priorHead, head, relation), nil
}

func (o *GitObserver) verifyStable(ctx context.Context, root string, common []byte, commonDirectoryIdentity string, gitDir []byte, worktreeGitDirectoryIdentity string, branch gitCommandResult, head []byte, status []porcelainRecord) error {
	checks := []struct {
		command  gitReadCommand
		expected []byte
	}{{gitShowCommonDirectory, common}, {gitShowWorktreeGitDirectory, gitDir}, {gitShowHead, head}}
	for _, check := range checks {
		result, err := o.runner.run(ctx, check.command, root, "")
		if err != nil {
			return err
		}
		if result.exitCode != 0 || !bytes.Equal(result.stdout, check.expected) {
			return ErrInconsistentWorktree
		}
	}
	commonDirectoryPath, err := canonicalGitDirectory(common)
	if err != nil {
		return ErrInconsistentWorktree
	}
	secondCommonIdentity, err := gitDirectoryIdentity(commonDirectoryPath)
	if err != nil || secondCommonIdentity != commonDirectoryIdentity {
		return ErrInconsistentWorktree
	}
	gitDirectoryPath, err := canonicalGitDirectory(gitDir)
	if err != nil {
		return ErrInconsistentWorktree
	}
	secondIdentity, err := gitDirectoryIdentity(gitDirectoryPath)
	if err != nil || secondIdentity != worktreeGitDirectoryIdentity {
		return ErrInconsistentWorktree
	}
	secondBranch, err := o.runner.run(ctx, gitShowBranch, root, "")
	if err != nil {
		return err
	}
	if secondBranch.exitCode != branch.exitCode || !bytes.Equal(secondBranch.stdout, branch.stdout) {
		return ErrInconsistentWorktree
	}
	secondStatus, err := o.runner.run(ctx, gitShowStatus, root, "")
	if err != nil {
		return err
	}
	if secondStatus.exitCode != 0 {
		return ErrInconsistentWorktree
	}
	records, err := parsePorcelainV2(secondStatus.stdout)
	if err != nil || ensureStatusUnchanged(status, records) != nil {
		return ErrInconsistentWorktree
	}
	return nil
}

func observedBranch(result gitCommandResult) (*string, bool, error) {
	switch result.exitCode {
	case 0:
		branch, err := parseSingleLine(result.stdout)
		if err != nil {
			return nil, false, ErrGitObservation
		}
		return &branch, false, nil
	case 1:
		return nil, true, nil
	default:
		return nil, false, ErrGitObservation
	}
}

func parseObjectLine(output []byte) (string, error) {
	value, err := parseSingleLine(output)
	if err != nil || !validGitObjectID(value) {
		return "", ErrGitObservation
	}
	return value, nil
}

type gitCommandRunner struct {
	timeout     time.Duration
	outputLimit int64
}
type gitCommandResult struct {
	stdout   []byte
	exitCode int
}

func (r gitCommandRunner) run(ctx context.Context, command gitReadCommand, repositoryPath, value string) (gitCommandResult, error) {

	args, ok := command.arguments(repositoryPath, value)
	if !ok {
		return gitCommandResult{}, ErrGitObservation
	}
	timeoutContext, timeoutCancel := context.WithTimeout(ctx, r.timeout)
	commandContext, commandCancel := context.WithCancel(timeoutContext)
	defer timeoutCancel()
	defer commandCancel()
	var stdout, stderr bytes.Buffer
	capture := boundedCommandCapture{remaining: r.outputLimit, cancel: commandCancel}
	cmd := exec.CommandContext(commandContext, gitExecutable, args...)
	configureGitCommand(cmd)
	cmd.Env = gitEnvironment(os.Environ())
	cmd.Stdout = capture.writer(&stdout)
	cmd.Stderr = capture.writer(&stderr)
	runErr := cmd.Run()
	if capture.limitExceeded() {
		return gitCommandResult{}, ErrGitOutputLimit
	}
	if err := ctx.Err(); err != nil {
		return gitCommandResult{}, err
	}
	if errors.Is(timeoutContext.Err(), context.DeadlineExceeded) {
		return gitCommandResult{}, ErrGitCommandTimeout
	}
	result := gitCommandResult{stdout: append([]byte(nil), stdout.Bytes()...)}
	if runErr == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(runErr, &exitError) {
		result.exitCode = exitError.ExitCode()
		return result, nil
	}
	return gitCommandResult{}, fmt.Errorf("%w: start Git", ErrGitObservation)
}

func (command gitReadCommand) arguments(repositoryPath, value string) ([]string, bool) {
	args := []string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-C", repositoryPath}
	pair := func() (string, string, bool) {
		left, right, ok := strings.Cut(value, "\x00")
		return left, right, ok && left != "" && right != ""
	}
	switch command {
	case gitShowWorktreeRoot:
		if value != "" {
			return nil, false
		}
		return append(args, "rev-parse", "--path-format=absolute", "--show-toplevel"), true
	case gitShowCommonDirectory:
		if value != "" {
			return nil, false
		}
		return append(args, "rev-parse", "--path-format=absolute", "--git-common-dir"), true
	case gitShowWorktreeGitDirectory:
		if value != "" {
			return nil, false
		}
		return append(args, "rev-parse", "--path-format=absolute", "--absolute-git-dir"), true
	case gitShowBranch:
		if value != "" {
			return nil, false
		}
		return append(args, "symbolic-ref", "--quiet", "--short", "HEAD"), true
	case gitShowWholeIndex:
		if value != "" {
			return nil, false
		}
		return append(args, "ls-files", "--stage", "-v", "-z"), true
	case gitShowBranchRef:
		if !validBranchRefName(value) {
			return nil, false
		}
		return append(args, "rev-parse", "--verify", "--quiet", "refs/heads/"+value+"^{commit}"), true
	case gitShowHead:
		if value != "" {
			return nil, false
		}
		return append(args, "rev-parse", "--verify", "HEAD"), true
	case gitShowHeadTree:
		if value != "" {
			return nil, false
		}
		return append(args, "rev-parse", "--verify", "HEAD^{tree}"), true
	case gitShowStatus:
		if value != "" {
			return nil, false
		}
		return append(args, "status", "--porcelain=v2", "--untracked-files=all", "--ignore-submodules=none", "--no-renames", "-z"), true
	case gitShowLocalBase:
		if !validBranchRefName(value) {
			return nil, false
		}
		return append(args, "rev-parse", "--verify", "refs/heads/"+value+"^{commit}"), true
	case gitShowRemoteBase:
		remote, branch, ok := pair()
		if !ok {
			return nil, false
		}
		return append(args, "rev-parse", "--verify", "refs/remotes/"+remote+"/"+branch+"^{commit}"), true
	case gitShowTaskSurface:
		if !validGitObjectID(value) {
			return nil, false
		}
		return append(args, "diff", "--no-ext-diff", "--cached", "--name-status", "--no-renames", "-z", value, "--"), true
	case gitShowWorktreeDelta:
		if value != "" {
			return nil, false
		}
		return append(args, "diff", "--no-ext-diff", "--name-status", "--no-renames", "-z", "--"), true
	case gitShowBaseEntries, gitShowIndexEntries:
		parts := strings.Split(value, "\x00")
		base := ""
		if command == gitShowBaseEntries {
			if len(parts) < 2 || !validGitObjectID(parts[0]) {
				return nil, false
			}
			base = parts[0]
			parts = parts[1:]
		}
		if len(parts) == 0 || len(parts) > 32 {
			return nil, false
		}
		for _, path := range parts {
			if path == "" {
				return nil, false
			}
		}
		args = append(args, "--literal-pathspecs")
		if command == gitShowBaseEntries {
			args = append(args, "ls-tree", "-z", base, "--")
		} else {
			args = append(args, "ls-files", "--stage", "-v", "-z", "--")
		}
		return append(args, parts...), true
	case gitIsAncestor:
		left, right, ok := pair()
		if !ok || !validGitObjectID(left) || !validGitObjectID(right) {
			return nil, false
		}
		return append(args, "merge-base", "--is-ancestor", left, right), true
	case gitShowMergeCommits:
		left, right, ok := pair()
		if !ok || !validGitObjectID(left) || !validGitObjectID(right) {
			return nil, false
		}
		return append(args, "rev-list", "--min-parents=2", left+".."+right), true
	default:
		return nil, false
	}
}

func gitEnvironment(environment []string) []string {
	clean := make([]string, 0, len(environment)+3)
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if !found || strings.HasPrefix(key, "GIT_") || key == "LC_ALL" || key == "LANG" {
			continue
		}
		clean = append(clean, entry)
	}
	return append(clean, "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C", "LANG=C")
}

type boundedCommandCapture struct {
	mu        sync.Mutex
	remaining int64
	exceeded  bool
	cancel    context.CancelFunc
}

func (capture *boundedCommandCapture) writer(destination *bytes.Buffer) *boundedCommandWriter {
	return &boundedCommandWriter{capture: capture, destination: destination}
}
func (capture *boundedCommandCapture) limitExceeded() bool {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.exceeded
}

type boundedCommandWriter struct {
	capture     *boundedCommandCapture
	destination *bytes.Buffer
}

func (writer *boundedCommandWriter) Write(value []byte) (int, error) {
	writer.capture.mu.Lock()
	defer writer.capture.mu.Unlock()
	if writer.capture.exceeded {
		return len(value), nil
	}
	if int64(len(value)) > writer.capture.remaining {
		writer.capture.exceeded = true
		writer.capture.cancel()
		return len(value), nil
	}
	writer.capture.remaining -= int64(len(value))
	_, _ = writer.destination.Write(value)
	return len(value), nil
}
