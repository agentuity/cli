import logging
from agentuity.server import autostart

if __name__ == "__main__":
    logging.basicConfig(
        level=logging.DEBUG,
    )
    autostart()
