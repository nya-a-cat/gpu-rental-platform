#!/usr/bin/env python3
import argparse
import hashlib
import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


class State:
    def __init__(self, report_path: Path, body_path: Path) -> None:
        self.report_path = report_path
        self.body_path = body_path
        self.lock = threading.Lock()
        self.body: bytes | None = None
        self.metadata: dict[str, str] = {}
        self.path = ""
        self.retention_mode = ""
        self.retention_until = ""
        self.authorization_v4 = False
        self.put_count = 0
        self.head_count = 0

    def write_report(self) -> None:
        required_metadata = {
            "x-amz-meta-sha256",
            "x-amz-meta-row-count",
            "x-amz-meta-period-start",
            "x-amz-meta-period-end",
            "x-amz-meta-schema",
        }
        metadata_keys = sorted(self.metadata)
        passed = (
            self.put_count == 1
            and self.body is not None
            and self.path.startswith("/audit/")
            and self.retention_mode in {"GOVERNANCE", "COMPLIANCE"}
            and bool(self.retention_until)
            and self.authorization_v4
            and required_metadata.issubset(self.metadata)
        )
        report = {
            "status": "passed" if passed else "failed",
            "path": self.path,
            "putCount": self.put_count,
            "headCount": self.head_count,
            "bytes": len(self.body or b""),
            "sha256": hashlib.sha256(self.body or b"").hexdigest(),
            "retentionMode": self.retention_mode,
            "retentionDatePresent": bool(self.retention_until),
            "authorizationV4": self.authorization_v4,
            "metadataKeys": metadata_keys,
        }
        temporary = self.report_path.with_suffix(".tmp")
        temporary.write_text(json.dumps(report, separators=(",", ":")) + "\n", encoding="utf-8")
        os.replace(temporary, self.report_path)


def handler_factory(state: State):
    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def do_GET(self) -> None:
            if self.path.endswith("?location"):
                body = b'<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>'
                self.send_response(200)
                self.send_header("Content-Type", "application/xml")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
            self.send_error(404)

        def do_HEAD(self) -> None:
            with state.lock:
                state.head_count += 1
                if state.body is None or self.path != state.path:
                    self.send_response(404)
                    self.send_header("x-amz-request-id", "fixture-request")
                    self.send_header("Content-Length", "0")
                    self.end_headers()
                    return
                self.send_response(200)
                self.send_header("Content-Length", str(len(state.body)))
                self.send_header("ETag", '"fixture-etag"')
                for key, value in state.metadata.items():
                    self.send_header(key, value)
                self.end_headers()

        def do_PUT(self) -> None:
            length = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(length)
            with state.lock:
                state.body = body
                state.body_path.write_bytes(body)
                state.path = self.path
                state.metadata = {
                    key.lower(): value
                    for key, value in self.headers.items()
                    if key.lower().startswith("x-amz-meta-")
                }
                state.retention_mode = self.headers.get("X-Amz-Object-Lock-Mode", "")
                state.retention_until = self.headers.get("X-Amz-Object-Lock-Retain-Until-Date", "")
                state.authorization_v4 = self.headers.get("Authorization", "").startswith("AWS4-HMAC-SHA256 ")
                state.put_count += 1
                state.write_report()
            self.send_response(200)
            self.send_header("ETag", '"fixture-etag"')
            self.send_header("Content-Length", "0")
            self.end_headers()

        def log_message(self, _format: str, *_args: object) -> None:
            return

    return Handler


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--report", required=True, type=Path)
    parser.add_argument("--body", required=True, type=Path)
    parser.add_argument("--port-file", required=True, type=Path)
    args = parser.parse_args()

    state = State(args.report, args.body)
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler_factory(state))
    args.port_file.write_text(str(server.server_port), encoding="utf-8")
    server.serve_forever()


if __name__ == "__main__":
    main()
