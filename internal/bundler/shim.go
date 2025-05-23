package bundler

var jsheader = "/* DO NOT EDIT - GENERATED CODE */\n"

var jsshim = `// Shim for dynamic requires of Node.js built-in modules
import { createRequire as __agentuity_createRequire } from 'module';
const require = __agentuity_createRequire(import.meta.url);
import { fileURLToPath as __agentuity_fileURLToPath } from 'url';
import { dirname as __agentuity_dirname } from 'path';
import { readFileSync as __agentuity_readFileSync, existsSync as __agentuity_existsSync } from 'fs';

const __filename = __agentuity_fileURLToPath(import.meta.url);
const __dirname = __agentuity_dirname(__filename);

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
};
`

// NOTE: this shim is only used in bun since node has built-in source map support
var sourceMapShim = `
(function () {
const { SourceMapConsumer: __agentuity_SourceMapConsumer } = require('source-map-js');
const { join: __agentuity_join } = require('path');
const __prepareStackTrace = Error.prepareStackTrace;
const __cachedSourceMap = {};
function getSourceMap(filename) {
	if (filename in __cachedSourceMap) {
		return __cachedSourceMap[filename];
	}
	if (!__agentuity_existsSync(filename)) {
		return null;
	}
	const sm = new __agentuity_SourceMapConsumer(__agentuity_readFileSync(filename).toString());
	__cachedSourceMap[filename] = sm;
	return sm;
}
const frameRegex = /(.+)\((.+):(\d+):(\d+)\)$/;
Error.prepareStackTrace = function (err, stack) {
	try {
		const _stack = __prepareStackTrace(err, stack);
		const tok = _stack.split('\n');
		const lines = [];
		for (const t of tok) {
			if (t.includes('.agentuity/') && frameRegex.test(t)) {
				const parts = frameRegex.exec(t);
				if (parts.length === 5) {
					const filename = parts[2];
					const sm = getSourceMap(filename+'.map');
					if (sm) {
						const lineno = parts[3];
						const colno = parts[4];
						const pos = sm.originalPositionFor({
							line: +lineno,
							column: +colno,
						})
						if (pos && pos.source) {
							const startIndex = filename.indexOf('.agentuity/');
							const offset = filename.includes('../node_modules/') ? 11 : 0;
							const basedir = filename.substring(0, startIndex + offset);
							const sourceOffset = pos.source.indexOf('src/');
							const source = pos.source.substring(sourceOffset);
							const newfile = __agentuity_join(basedir, source);
							const newline = parts[1] + '(' + newfile + ':' + pos.line + ':' + pos.column + ')';
							lines.push(newline);
							continue;
						}
					}
				}
			}
			lines.push(t);
		}
		return lines.join('\n');
	} catch (e) {
		return stack;
	}
};
})();
`
