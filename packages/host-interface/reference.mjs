import { readFile } from 'node:fs/promises';
import { join } from 'node:path';

/**
 * Read installed method descriptions without opening Task storage or invoking
 * the operation. Each package carries its own complete generated reference.
 */
export async function describeInterface(packageRoot, name) {
  const entries = JSON.parse(await readFile(join(packageRoot, 'contracts', 'interfaces.json'), 'utf8'));
  if (!Array.isArray(entries)) throw new Error('Invalid packaged interface reference');
  if (name === undefined) return entries.map(({ name, description }) => ({ name, description }));
  const contract = entries.find(entry => entry.name === name);
  if (!contract) throw new Error('Unknown interface: ' + name);
  return contract;
}

export function interfaceHelpRequest(args) {
  if (!Array.isArray(args) || !args.includes('--help')) return null;
  const at = args.indexOf('--help');
  if (at < 1 || at > 2 || args.length > at + 2 || args[at + 1] !== undefined && args[at + 1] !== '--json') return null;
  if (at === 2 && args[0] === 'call') return { name: args[1] };
  if (at === 2 && args[0] === 'check' && args[1] === 'run') return { name: 'check.run' };
  if (at === 2 && args[0] === 'host-check') return { name: 'host-check.' + args[1] };
  if (args[0] === 'hook' && at === 2) return { group: 'hook', operation: args[1] };
  if (args[0] === 'host-launch') return { group: 'host-launch', operation: at === 2 ? args[1] : undefined };
  if (at === 1 && ['status','setup','remove'].includes(args[0])) return { group: 'lifecycle', operation: args[0] };
  return null;
}
