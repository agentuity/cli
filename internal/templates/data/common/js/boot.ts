import { runner } from "@agentuity/sdk";

runner(true, import.meta.dirname).catch((err) => {
	console.error(err);
	process.exit(1);
});