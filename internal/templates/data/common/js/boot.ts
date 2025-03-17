import { runner } from "@agentuity/sdk";

if (!process.env.AGENTUITY_API_KEY) {
	console.error("\x1b[31m[ERROR] AGENTUITY_API_KEY is not set. This should have been set automatically by the Agentuity CLI or picked up from the .env file.\x1b[0m");
	const cmd = process.env._ || "";
	if (cmd.endsWith("node")) {
		console.error(
			"\x1b[31m[ERROR] Re-run the command with `node --env-file .env index.ts`\x1b[0m",
		);
	}
	process.exit(1);
}

if (!process.env.AGENTUITY_URL) {
	console.warn("\x1b[31m[WARN] You are running this agent outside of the Agentuity environment. Any automatic Agentuity features will be disabled.\x1b[0m");
	if (process.isBun) {
		console.warn("\x1b[31m[WARN] Recommend running `agentuity dev` to run your project locally instead of bun run start.\x1b[0m");
	} else {
		console.warn("\x1b[31m[WARN] Recommend running `agentuity dev` to run your project locally instead of npm start.\x1b[0m");
	}
}

runner(true, import.meta.dirname).catch((err) => {
	if (err instanceof Error) {
		console.error(err.message);
		console.error(err.stack);
	} else {
		console.error(err);
	}
	process.exit(1);
});