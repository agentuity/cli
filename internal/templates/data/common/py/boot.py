import logging
from agentuity import autostart

if __name__ == "__main__":
    logging.basicConfig(
        level=logging.INFO,
        format='[%(levelname)5.5s] %(message)s',
    )
    autostart()
