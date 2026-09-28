import asyncio
import sys

from core.process_supervisor import ProcessSupervisor


def test_supervisor_tracks_short_lived_process_exit(tmp_path):
    async def run():
        supervisor = ProcessSupervisor()
        started = await supervisor.start([sys.executable, "-c", "print('ready')"], tmp_path)
        if started:
            await supervisor._read_task
        return started, supervisor.status, supervisor.get_logs()

    started, status, logs = asyncio.run(run())

    assert started is True
    assert status == "STOPPED"
    assert any("ready" in line for line in logs)


def test_supervisor_stop_waits_for_process_exit(tmp_path):
    async def run():
        supervisor = ProcessSupervisor()
        started = await supervisor.start(
            [sys.executable, "-c", "import sys,time; print('ready', flush=True); sys.stdin.readline(); sys.exit(0)"],
            tmp_path,
        )
        stopped = await supervisor.stop("quit", timeout=3)
        return started, stopped, supervisor.is_running(), supervisor.status

    started, stopped, running, status = asyncio.run(run())

    assert started is True
    assert stopped is True
    assert running is False
    assert status == "STOPPED"
