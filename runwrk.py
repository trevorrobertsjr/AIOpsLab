# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.

"""MongoDB storage user unregistered problem in the HotelReservation application."""

from typing import Any

from aiopslab.orchestrator.tasks import *
from aiopslab.orchestrator.evaluators.quantitative import *
from aiopslab.service.kubectl import KubeCtl
from aiopslab.service.apps.hotelres import HotelReservation
from aiopslab.generators.workload.wrk import Wrk
from aiopslab.generators.fault.inject_app import ApplicationFaultInjector
from aiopslab.session import SessionItem
from aiopslab.paths import TARGET_MICROSERVICES

from aiopslab.orchestrator.problems.misconfig_app.helpers import get_frontend_url

app = HotelReservation()
frontend_url = get_frontend_url(app)
payload_script = (
            TARGET_MICROSERVICES
            / "hotelReservation/wrk2/scripts/hotel-reservation/mixed-workload_type_1.lua"
        )
wrk = Wrk(rate=10, dist="exp", connections=2, duration=10, threads=2)
wrk.start_workload(
    payload_script=payload_script,
    url=f"{frontend_url}",
)


