import { mkdir, readdir, lstat } from "node:fs/promises";
import { isAbsolute, join, relative, sep } from "node:path";
import { inspectResource, writeOwnedJSON } from "./ownership.mjs";
import { loadPetPlatform } from "./platform.mjs";
import { readRun } from "./journal.mjs";

/**
 * Data cleanup enumerates user resources, never their parent installation roots.
 * The desktop executable and registration receipts survive both data operations.
 */
export async function observeCleanupResources(paths, resources) {
  for (const resource of [resources.defaultData, resources.explicitData].filter(resource => resource?.exists)) {
    if (await lstat(join(resource.path, ".git")).catch(error => { if (error.code !== "ENOENT") throw error; })) throw new Error(`Task data directory is a Git workspace: ${resource.path}`);
  }
  const petPlatform = await loadPetPlatform(paths.platform, paths.arch);
  const petEntries = resources.pet.exists ? await readdir(paths.petDirectory) : [];
  const petData = petEntries.includes(petPlatform.PET_APPLICATION_NAME)
    ? await Promise.all(petEntries.filter(name => name !== petPlatform.PET_APPLICATION_NAME).sort().map(name => inspectResource(join(paths.petDirectory, name), `pet-${name}`)))
    : resources.pet.exists ? [resources.pet] : [];
  const completedRuns = [];
  const runDirectory = await inspectResource(paths.runsDirectory, "maintenance-records");
  if (runDirectory.exists) {
    for (const name of (await readdir(paths.runsDirectory)).sort()) {
      if (!name.endsWith(".json")) throw new Error("Unknown maintenance record in cleanup scope");
      const record = await readRun(paths, name.slice(0, -5));
      if (record?.next_step === "complete" && !record.completed_action_ids.includes("manager.cleanup")) completedRuns.push(await inspectResource(join(paths.runsDirectory, name), `run-${name.slice(0, -5)}`));
    }
  }
  return { ...resources, petData, completedRuns };
}

export function assertCleanupLocations(paths, targets, packageRoots = []) {
  const parents = [paths.homeDirectory, paths.managerRoot, paths.configurationDirectory, paths.petDirectory].filter(Boolean);
  const retained = [join(paths.productRoot, "registrations"), join(paths.managerRoot, "profiles"), paths.trashDirectory, ...packageRoots].filter(Boolean);
  for (const target of targets.filter(target => target.requiresExplicitConfirmation)) {
    for (const protectedPath of [...parents, ...retained]) {
      if (contains(target.path, protectedPath) || retained.includes(protectedPath) && contains(protectedPath, target.path)) throw new Error(`Confirmed data directory overlaps a retained installation resource: ${protectedPath}`);
    }
  }
  for (let i = 0; i < targets.length; i++) for (let j = i + 1; j < targets.length; j++) {
    if (contains(targets[i].path, targets[j].path) || contains(targets[j].path, targets[i].path)) throw new Error("Cleanup directories overlap; select a separate Task data directory");
  }
}

export async function initializeEmptyData(paths, targets) {
  const directories = new Set([paths.defaultDataDirectory, ...targets.filter(target => target.label === "default-data" || target.requiresExplicitConfirmation).map(target => target.path)]);
  for (const directory of directories) {
    await mkdir(directory, { recursive: true, mode: 0o700 });
    const resource = await inspectResource(directory, "empty-data");
    if (resource.kind !== "directory") throw new Error("Fresh data path is not a directory");
    if ((await readdir(directory)).length !== 0) throw new Error(`Data was written during cleanup: ${directory}`);
  }
  await writeOwnedJSON(paths.configurationPath, {}, { root: paths.configurationDirectory, enforcePrivateModes: paths.enforcePrivateModes });
  return { changed: true, completedSteps: ["manager.initialize_configuration", "manager.initialize_data"] };
}
function contains(parent, child) { const offset = relative(parent, child); return offset === "" || offset !== ".." && !offset.startsWith(`..${sep}`) && !isAbsolute(offset); }
