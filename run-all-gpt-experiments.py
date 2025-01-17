import asyncio
import time
from datetime import datetime
import pytz  # For timezone handling
import os

from aiopslab.orchestrator import Orchestrator
from clients.utils.llm import GPT4Turbo
from clients.utils.templates import DOCS_SHELL_ONLY

# Define Pacific Time timezone
PACIFIC_TIMEZONE = pytz.timezone("US/Pacific")

class Agent:
    def __init__(self):
        self.history = []
        self.llm = GPT4Turbo()

    def init_context(self, problem_desc: str, instructions: str, apis: str):
        """Initialize the context for the agent."""

        self.shell_api = self._filter_dict(apis, lambda k, _: "exec_shell" in k)
        self.submit_api = self._filter_dict(apis, lambda k, _: "submit" in k)
        stringify_apis = lambda apis: "\n\n".join(
            [f"{k}\n{v}" for k, v in apis.items()]
        )

        self.system_message = DOCS_SHELL_ONLY.format(
            prob_desc=problem_desc,
            shell_api=stringify_apis(self.shell_api),
            submit_api=stringify_apis(self.submit_api),
        )

        self.task_message = instructions

        self.history.append({"role": "system", "content": self.system_message})
        self.history.append({"role": "user", "content": self.task_message})

    async def get_action(self, input) -> str:
        """Wrapper to interface the agent with OpsBench.

        Args:
            input (str): The input from the orchestrator/environment.

        Returns:
            str: The response from the agent.
        """
        self.history.append({"role": "user", "content": input})
        response = self.llm.run(self.history)
        self.history.append({"role": "assistant", "content": response[0]})
        return response[0]

    def _filter_dict(self, dictionary, filter_func):
        return {k: v for k, v in dictionary.items() if filter_func(k, v)}

if __name__ == "__main__":
    agent = Agent()
    orchestrator = Orchestrator()
    orchestrator.register_agent(agent, name="gpt-w-shell")

    pids = [
        "revoke_auth_mongodb-detection-1",
        "revoke_auth_mongodb-localization-1",
        "revoke_auth_mongodb-analysis-1",
        "revoke_auth_mongodb-mitigation-1",
        "revoke_auth_mongodb-detection-2",
        "revoke_auth_mongodb-localization-2",
        "revoke_auth_mongodb-analysis-2",
        "revoke_auth_mongodb-mitigation-2",
        "user_unregistered_mongodb-detection-1",
        "user_unregistered_mongodb-localization-1",
        "user_unregistered_mongodb-analysis-1",
        "user_unregistered_mongodb-mitigation-1",
        "user_unregistered_mongodb-detection-2",
        "user_unregistered_mongodb-localization-2",
        "user_unregistered_mongodb-analysis-2",
        "user_unregistered_mongodb-mitigation-2",
        "misconfig_app_hotel_res-detection-1",
        "misconfig_app_hotel_res-localization-1",
        "misconfig_app_hotel_res-analysis-1",
        "misconfig_app_hotel_res-mitigation-1",
        "pod_failure_hotel_res-detection-1",
        "pod_failure_hotel_res-localization-1",
        # "network_loss_hotel_res-detection-1", # uses helm chart
        # "network_loss_hotel_res-localization-1", # uses helm chart
        "noop_detection_hotel_reservation-1",
    ]

    log_filename = "pid_execution_log.txt"

    with open(log_filename, "w") as log_file:
        for pid in pids:
            start_time = datetime.now(PACIFIC_TIMEZONE)
            formatted_start_time = start_time.strftime("%Y-%m-%d %H:%M:%S %Z")
            log_file.write(f"PID: {pid}, Start Time: {formatted_start_time}\n")
            print(f"[{formatted_start_time}] Starting execution for PID: {pid}")

            problem_desc, instructs, apis = orchestrator.init_problem(pid)
            agent.init_context(problem_desc, instructs, apis)
            asyncio.run(orchestrator.start_problem(max_steps=10))

            end_time = datetime.now(PACIFIC_TIMEZONE)
            formatted_end_time = end_time.strftime("%Y-%m-%d %H:%M:%S %Z")
            log_file.write(f"PID: {pid}, End Time: {formatted_end_time}\n\n")
            print(f"[{formatted_end_time}] Finished execution for PID: {pid}. Sleeping for 30 seconds.")
            time.sleep(30)

    timestamp = datetime.now(PACIFIC_TIMEZONE).strftime("%Y%m%d%H%M%S")
    new_log_filename = f"pid_execution_log_{timestamp}.txt"
    os.rename(log_filename, new_log_filename)
    print(f"Log file renamed to {new_log_filename}.")
