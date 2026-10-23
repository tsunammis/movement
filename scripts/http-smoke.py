"""Exercise a built Movement binary using an isolated temporary repository."""
import json
import selectors
import signal
import subprocess
import sys
import tempfile
import urllib.request

binary = sys.argv[1]
with tempfile.TemporaryDirectory(prefix="movement-http-") as directory:
    command = [binary, "--data-dir", directory]
    subprocess.run(command + ["init"], check=True, capture_output=True)
    token = subprocess.check_output(command + ["auth", "token"], text=True).strip()
    process = subprocess.Popen(command + ["serve", "--listen", "127.0.0.1:0"],
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        with selectors.DefaultSelector() as selector:
            selector.register(process.stderr, selectors.EVENT_READ)
            assert selector.select(10), "server startup timed out"
        startup = process.stderr.readline().strip()
        assert startup.startswith("Movement API: "), startup
        url = startup.removeprefix("Movement API: ")
        data = json.dumps({"operation_id": "http-smoke", "actor": "smoke",
                           "data": {"title": "HTTP integration", "status": "ready"}}).encode()
        request = urllib.request.Request(url + "/api/v1/tasks", data=data,
                  headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
        with urllib.request.urlopen(request, timeout=5) as response:
            receipt = json.load(response)
        task = json.loads(subprocess.check_output(command + ["task", "get", receipt["task_id"], "--json"]))
        assert task["title"] == "HTTP integration"
        process.send_signal(signal.SIGTERM)
        assert process.wait(timeout=10) == 0
        assert not process.stdout.read(), "unexpected stdout"
        print("PASS: loopback HTTP creation, independent CLI readback, graceful shutdown")
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
