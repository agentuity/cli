import os
import json
import signal
import sys
import base64
import importlib.util
from http.server import HTTPServer
from http.server import BaseHTTPRequestHandler

# Load agents from local config file
try:
    config_path = os.path.join(os.getcwd(), ".agentuity", "config.json")
    if os.path.exists(config_path):
        with open(config_path, "r") as config_file:
            config_data = json.load(config_file)
            agents = config_data.get("agents", [])
            agents_by_id = {agent["id"]: agent for agent in agents}
            print(f"Loaded {len(agents)} agents from {config_path}")
    else:
        print(f"No agent configuration found at {config_path}")
        sys.exit(1)
except json.JSONDecodeError as e:
    print(f"Error parsing agent configuration: {e}")
    sys.exit(1)
except Exception as e:
    print(f"Error loading agent configuration: {e}")
    sys.exit(1)


class WebRequestHandler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        # Override to suppress log messages
        return

    def do_GET(self):
        # Check if the path is a health check
        print(f"Processing GET request: {self.path}")
        if self.path == "/_health":
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write("OK".encode("utf-8"))
        else:
            self.send_response(404)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write("Not Found".encode("utf-8"))

    def do_POST(self):
        # Extract the agent ID from the path (remove leading slash)
        agentId = self.path[1:]
        print(f"Processing request for agent: {agentId}")

        # Check if the agent exists in our map
        if agentId in agents_by_id:
            agent = agents_by_id[agentId]
            filename = agent["filename"]

            try:
                # Load the agent module dynamically
                spec = importlib.util.spec_from_file_location("agent_module", filename)
                if spec is None:
                    raise ImportError(f"Could not load spec for {filename}")

                agent_module = importlib.util.module_from_spec(spec)
                spec.loader.exec_module(agent_module)

                # Check if the module has a run function
                if hasattr(agent_module, "run") and callable(agent_module.run):
                    # Call the run function and get the response
                    response = agent_module.run()

                    # Send successful response
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.end_headers()

                    content_type = "text/plain"
                    # Base64 encode the payload
                    encoded_payload = base64.b64encode(
                        str(response).encode("utf-8")
                    ).decode("utf-8")

                    self.wfile.write(
                        json.dumps(
                            {
                                "contentType": content_type,
                                "payload": encoded_payload,
                                "metadata": {},
                            }
                        ).encode("utf-8")
                    )
                else:
                    raise ImportError(f"Module {filename} does not have a run function")

            except Exception as e:
                print(f"Error loading or running agent: {e}")
                self.send_response(500)
                self.send_header("Content-Type", "text/plain")
                self.end_headers()
                self.wfile.write(
                    str(f"Error loading or running agent: {str(e)}").encode("utf-8")
                )
        else:
            # Agent not found
            self.send_response(404)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()


def signal_handler(sig, frame):
    """Handle keyboard interrupt gracefully."""
    print("\nShutting down the server...")
    sys.exit(0)


if __name__ == "__main__":
    # Register signal handler for graceful shutdown
    signal.signal(signal.SIGINT, signal_handler)

    # Get port from environment variable or use default
    port = int(os.environ.get("PORT", 3500))

    print(f"Starting server on http://0.0.0.0:{port}")
    server = HTTPServer(("0.0.0.0", port), WebRequestHandler)

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down the server...")
    except Exception as e:
        print(f"Error: {e}")
    finally:
        server.server_close()
        print("Server stopped.")
