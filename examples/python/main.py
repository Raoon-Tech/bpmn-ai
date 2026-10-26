"""Minimal example: deploy a process and start a case using the REST API."""
import json
import urllib.request

BASE = "http://localhost:8080"


def request(method, path, body=None, headers=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    with urllib.request.urlopen(req) as r:
        return json.load(r)


# 1. Fetch a CSRF token (only needed when CSRF is enabled).
with urllib.request.urlopen(BASE + "/api/v1/csrf-token") as r:
    csrf = json.load(r)["csrf_token"]
headers = {"X-CSRF-Token": csrf}
# (also send the csrf_token cookie in real clients)

# 2. Deploy a process.
xml = open("testdata/simple_sequence.bpmn").read()
proc = request("POST", "/api/v1/processes", {"name": "Python demo", "bpmn_xml": xml}, headers)
print("process:", proc["id"])

# 3. Start a case.
case = request("POST", f"/api/v1/processes/{proc['id']}/start", {"variables": {"approved": True}}, headers)
print("case:", case["id"], case["status"])
PY