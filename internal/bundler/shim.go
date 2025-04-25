package bundler

var jsheader = "/* DO NOT EDIT - GENERATED CODE */\n"

var jsshim = `// Shim for dynamic requires of Node.js built-in modules
import { createRequire as __agentuity_createRequire } from 'module';
const require = __agentuity_createRequire(import.meta.url);

// List of Node.js built-in modules that might be dynamically required
const nodeBuiltins = [
	'perf_hooks', 'path', 'fs', 'util', 'os', 'crypto', 'http', 'https',
	'url', 'stream', 'zlib', 'events', 'buffer', 'assert',
	'net', 'module', 'buffer'
];
// List of Node.js built-in modules that are not supported
const excludeBuiltins = ['vm', 'worker_threads', 'dgram', 'dns', 'child_process', 'tls', 'tty'];

// Handle node: prefix as well
const nodeNamespaceBuiltins = nodeBuiltins.map(m => 'node:' + m);
const excludeNodeNamespaceBuiltins = excludeBuiltins.map(m => 'node:' + m);
const allBuiltins = [...nodeBuiltins, ...nodeNamespaceBuiltins];
const excludes = [...excludeBuiltins, ...excludeNodeNamespaceBuiltins];

globalThis.__require = (id) => {
	// Check to see if the module is in the exclude list
	if (excludes.includes(id)) {
		throw new Error('require of ' + id + ' is not supported');
	}
	// Check if it's a Node.js built-in module
	if (allBuiltins.includes(id)) {
		try {
			return require(id);
		} catch (e) {
			// If the module name has node: prefix, try without it
			if (id.startsWith('node:')) {
				return require(id.substring(5));
			}
			// If the module name doesn't have node: prefix, try with it
			return require('node:' + id);
		}
	}
	throw new Error('Dynamic require of ' + id + ' is not supported');
};`
