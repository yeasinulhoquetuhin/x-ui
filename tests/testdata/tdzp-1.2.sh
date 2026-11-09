#!/usr/bin/env bash
# Original TDZP 1.2 relay fixture, with only non-runtime menu code omitted.
set -euo pipefail
instance_exists() { [[ -f "$TDZP_BASE_DIR/instances/$1.conf" ]]; }
instance_get() {
    local PUBLIC_PORT BACKEND_PORT
    source "$TDZP_BASE_DIR/instances/$1.conf"
    printf '%s' "${!2}"
}
valid_port() { [[ "$1" =~ ^[0-9]+$ ]]; }
run_proxy() {
    local id=${1:-} public backend
    instance_exists "$id" || exit 1
    public=$(instance_get "$id" PUBLIC_PORT)
    backend=$(instance_get "$id" BACKEND_PORT)
    valid_port "$public" && valid_port "$backend" || exit 1

    exec python3 - "$public" "$backend" <<'PYTHON_PROXY'
import asyncio
import contextlib
import signal
import socket
import sys

PUBLIC_PORT = int(sys.argv[1])
BACKEND_PORT = int(sys.argv[2])
HEADER_END = b"\r\n\r\n"
RESPONSE = (
    b"HTTP/1.1 101 Switching Protocols\r\n"
    b"Upgrade: websocket\r\n"
    b"Connection: Upgrade\r\n\r\n"
)


def tune(writer):
    sock = writer.get_extra_info("socket")
    if sock is None:
        return
    with contextlib.suppress(OSError):
        sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
    with contextlib.suppress(OSError):
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_KEEPALIVE, 1)


async def close(writer):
    if writer is None:
        return
    writer.close()
    with contextlib.suppress(Exception):
        await writer.wait_closed()


async def read_header(reader):
    data = bytearray()
    while HEADER_END not in data:
        if len(data) >= 65536:
            raise ValueError("header too large")
        chunk = await asyncio.wait_for(reader.read(4096), timeout=8)
        if not chunk:
            raise ConnectionError("incomplete header")
        data.extend(chunk)
    end = data.index(HEADER_END) + 4
    return bytes(data[end:])


async def relay(reader, writer):
    try:
        while True:
            data = await reader.read(65536)
            if not data:
                break
            writer.write(data)
            await writer.drain()
    except (ConnectionError, OSError, asyncio.CancelledError):
        pass


async def client_handler(client_reader, client_writer):
    backend_writer = None
    try:
        tune(client_writer)
        pending = await read_header(client_reader)
        backend_reader, backend_writer = await asyncio.open_connection("127.0.0.1", BACKEND_PORT)
        tune(backend_writer)
        client_writer.write(RESPONSE)
        await client_writer.drain()
        if pending:
            backend_writer.write(pending)
            await backend_writer.drain()
        tasks = {
            asyncio.create_task(relay(client_reader, backend_writer)),
            asyncio.create_task(relay(backend_reader, client_writer)),
        }
        done, pending_tasks = await asyncio.wait(tasks, return_when=asyncio.FIRST_COMPLETED)
        for task in pending_tasks:
            task.cancel()
        await asyncio.gather(*done, *pending_tasks, return_exceptions=True)
    except (asyncio.TimeoutError, ConnectionError, OSError, ValueError):
        pass
    finally:
        await close(backend_writer)
        await close(client_writer)


async def main():
    server = await asyncio.start_server(
        client_handler,
        "0.0.0.0",
        PUBLIC_PORT,
        backlog=512,
        reuse_address=True,
        limit=131072,
    )
    stop = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGTERM, signal.SIGINT):
        with contextlib.suppress(NotImplementedError):
            loop.add_signal_handler(sig, stop.set)
    async with server:
        await stop.wait()


asyncio.run(main())
PYTHON_PROXY
}
shift
run_proxy "$1"
