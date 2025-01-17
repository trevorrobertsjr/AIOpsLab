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
app.delete()
