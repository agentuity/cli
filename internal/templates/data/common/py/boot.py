import logging
import os
import sys
from agentuity import autostart

if __name__ == "__main__":
    # Check if AGENTUITY_API_KEY is set
    if not os.environ.get("AGENTUITY_API_KEY"):
        print(
            "\033[31m[ERROR] AGENTUITY_API_KEY is not set. This should have been set automatically by the Agentuity CLI or picked up from the .env file.\033[0m"
        )
        if os.environ.get("_", "").endswith("uv") and os.path.exists(".env"):
            print(
                "\033[31m[ERROR] Re-run the command with `uv run --env-file .env server.py`\033[0m"
            )
        sys.exit(1)

    # Check if AGENTUITY_URL is set
    if not os.environ.get("AGENTUITY_URL"):
        print(
            "\033[31m[WARN] You are running this agent outside of the Agentuity environment. Any automatic Agentuity features will be disabled.\033[0m"
        )
        print(
            "\033[31m[WARN] Recommend running `agentuity dev` to run your project locally instead of python script.\033[0m"
        )

    # Setup logging after environment checks
    logging.basicConfig(
        level=logging.INFO,
        format="[%(levelname)-5.5s] %(message)s",
    )

    autostart()
