import { runner } from "@agentuity/sdk";

if (!process.env.AGENTUITY_API_KEY) {
	console.error("AGENTUITY_API_KEY is not set. This should have been set automatically by the Agentuity CLI or picked up from the .env file.");
	process.exit(1);
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