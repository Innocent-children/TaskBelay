import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, rm, realpath } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { inspectSourceRepository, prepareLocalBranch, createCliWorktree } from '../packages/host-workspace/worktree-lifecycle.mjs';

test('actual branch and worktree creation keep source tracking separate under inherited Git defaults',async t=>{
 const parent=await realpath(await mkdtemp(join(tmpdir(),'taskbelay-tracking-')));t.after(()=>rm(parent,{recursive:true,force:true}));
 const repo=join(parent,'repo'),remote=join(parent,'remote.git');await mkdir(repo);
 const git=(...args)=>execFileSync('git',args,{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim();
 git('init','--bare',remote);git('-C',repo,'init','-b','main');git('-C',repo,'config','user.name','Fixture');git('-C',repo,'config','user.email','fixture@localhost');git('-C',repo,'commit','--allow-empty','-m','fixture');git('-C',repo,'remote','add','origin',remote);git('-C',repo,'push','-u','origin','main');
 git('-C',repo,'config','branch.autoSetupMerge','always');git('-C',repo,'config','remote.pushDefault','origin');git('-C',repo,'config','push.default','current');
 const source=await inspectSourceRepository(repo);assert.equal(source.upstream,'refs/remotes/origin/main');
 const local=await prepareLocalBranch({repositoryPath:repo,workspaceMode:'new_branch',baseBranch:'main',targetBranch:'feature/local',baseCommit:source.head,sourceRepositoryIdentity:source.source_repository_identity,carryChanges:false});
 assert.equal(local.upstream,null);assert.deepEqual(local.push_configuration,{branch_push_remote:null,remote_push_default:'origin',push_default:'current',ide_target:'unknown'});
 const path=join(parent,'worktree');await createCliWorktree({repositoryPath:repo,worktreePath:path,baseCommit:source.head,targetBranch:'feature/worktree',sourceRepositoryIdentity:source.source_repository_identity});
 const worktree=await inspectSourceRepository(path);assert.equal(worktree.upstream,null);assert.equal(worktree.head,source.head);
 assert.equal(git('-C',repo,'for-each-ref','--format=%(upstream)','refs/heads/main'),'refs/remotes/origin/main');
});
