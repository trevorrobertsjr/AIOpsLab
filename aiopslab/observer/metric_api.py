# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.

import os
import random
from datetime import datetime
from typing import Union
from datetime import datetime, timedelta
from kubernetes import client

import pandas as pd
import pytz
from prometheus_api_client import PrometheusConnect

from aiopslab.observer import monitor_config, root_path, get_pod_list, get_services_list

normal_metrics = [
    # cpu
    "container_cpu_usage_seconds_total",
    "container_cpu_user_seconds_total",
    "container_cpu_system_seconds_total",
    "container_cpu_cfs_throttled_seconds_total",
    "container_cpu_cfs_throttled_periods_total",
    "container_cpu_cfs_periods_total",
    "container_cpu_load_average_10s",
    # memory
    "container_memory_cache",
    "container_memory_usage_bytes",
    "container_memory_working_set_bytes",
    "container_memory_rss",
    "container_memory_mapped_file",
    # spec
    "container_spec_cpu_period",
    "container_spec_cpu_quota",
    "container_spec_memory_limit_bytes",
    "container_spec_cpu_shares",
    # threads
    "container_threads",
    "container_threads_max"
    # network
    "container_network_receive_errors_total",
    "container_network_receive_packets_dropped_total",
    "container_network_receive_packets_total",
    "container_network_receive_bytes_total",
    "container_network_transmit_bytes_total",
    "container_network_transmit_errors_total",
    "container_network_transmit_packets_dropped_total",
    "container_network_transmit_packets_total",
]
istio_metrics = [
    # istio
    "istio_requests_total",
    "istio_request_duration_milliseconds_sum",
    "istio_request_bytes_sum",
    "istio_response_bytes_sum",
    "istio_request_messages_total",
    "istio_response_messages_total",
    "istio_tcp_sent_bytes_total",
    "istio_tcp_received_bytes_total",
    "istio_tcp_connections_opened_total",
    "istio_tcp_connections_closed_total",
]
network_metrics = [
    # network
    "container_network_receive_errors_total",
    "container_network_receive_packets_dropped_total",
    "container_network_receive_packets_total",
    "container_network_receive_bytes_total",
    "container_network_transmit_bytes_total",
    "container_network_transmit_errors_total",
    "container_network_transmit_packets_dropped_total",
    "container_network_transmit_packets_total",
]


def time_format_transform(time):
    # transform time data from int to datetime
    if isinstance(time, int):
        time = datetime.fromtimestamp(time)
    elif isinstance(time, str):
        time = int(time)
        time = datetime.fromtimestamp(time)
    return time

    # def istio_cmdb_id_format(metric):
    pod = metric["pod"]
    service = pod.split("-")[0]
    source_service = metric["source_canonical_service"]
    destination_service = metric["destination_canonical_service"]

    if source_service not in self.service_list and source_service != "unknown":
        return ""
    if destination_service not in self.service_list and source_service != "unknown":
        return ""

    if service == source_service:
        cmdb_id = ".".join([pod, "source", source_service, destination_service])
    else:
        cmdb_id = ".".join([pod, "destination", source_service, destination_service])
    return cmdb_id


def network_kpi_name_format(metric):
    kpi_name = metric["__name__"]

    if "interface" in metric:
        kpi_name = ".".join([kpi_name, metric["interface"]])

    return kpi_name

    # def istio_kpi_name_format(metric):
    kpi_name = metric["__name__"]
    if "request_protocol" in metric:
        protocol = metric["request_protocol"]
        response_code = ""
        if "response_code" in metric:
            response_code = metric["response_code"]

        grpc_response_status = ""
        if "grpc_response_status" in metric:
            grpc_response_status = metric["grpc_response_status"]

        if protocol == "tcp":
            response_flag = metric["response_flags"]
            kpi_name = ".".join([kpi_name, response_flag])
        else:
            kpi_name = ".".join(
                [kpi_name, protocol, response_code, grpc_response_status]
            )
    return kpi_name


class PrometheusAPI:
    # disable_ssl – (bool) if True, will skip prometheus server's http requests' SSL certificate
    def __init__(self, url: str, namespace: str):
        self.client = PrometheusConnect(url, disable_ssl=True)
        self.namespace = namespace
        self.pod_list, self.service_list = self.initialize_pod_and_service_lists(
            namespace
        )

    def initialize_pod_and_service_lists(self, custom_namespace=None):
        namespace = custom_namespace or monitor_config["namespace"]
        v1 = client.CoreV1Api()
        pod_list = [
            pod
            for pod in get_pod_list(v1, namespace=namespace)
            if not pod.startswith("loadgenerator-") and not pod.startswith("redis-cart")
        ]
        service_list = get_services_list(v1, namespace=namespace)
        return pod_list, service_list

    # start_time: Union[int, datetime]
    # The start_time can be either int or datetime or string
    def query_range(
        self,
        metric_name: str,
        pod: str,
        start_time: Union[int, datetime, str],
        end_time: Union[int, datetime, str],
        namespace: str = "default",
        step: int = 60,
    ):
        start_time = time_format_transform(start_time)
        end_time = time_format_transform(end_time)
        interface = "eth0"
        if metric_name.endswith("_total") or metric_name in [
            "container_last_seen",
            "container_memory_cache",
            "container_memory_max_usage_bytes",
        ]:
            if metric_name in network_metrics:
                query = (
                    f"irate({metric_name}{{pod='{pod}', interface='{interface}'}}[5m])"
                )
            # prometheus query
            else:
                query = (
                    f"rate({metric_name}{{pod=~'{pod}', namespace='{namespace}'}}[5m])"
                )
        else:
            query = f"{metric_name}{{pod=~'{pod}', namespace='{namespace}'}}"
        data_raw = self.client.custom_query_range(
            query, start_time, end_time, step=step
        )

        if len(data_raw) == 0:
            return {"error": f"No data found for metric {metric_name} and pod {pod}"}
        else:
            data = []
            for item in data_raw[0]["values"]:
                date_time = datetime.fromtimestamp(int(item[0]))
                date_time = date_time.astimezone(pytz.timezone("Asia/Shanghai"))
                float_value = round(
                    float(item[1]), 3
                )  # float value is needed to be able to add it to the list as a whole.
                data.append({"time": date_time, "value": float_value})
            return data

    def export_all_metrics(self, start_time, end_time, save_path, step=15):
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        save_path = os.path.join(save_path, f"metric_{timestamp}")
        if not os.path.exists(save_path):
            os.makedirs(save_path)
        # namespace = monitor_config["namespace"]
        # container metrics
        container_save_path = os.path.join(save_path, "container")
        os.makedirs(container_save_path, exist_ok=True)
        # istio metrics
        istio_save_path = os.path.join(save_path, "istio")
        os.makedirs(istio_save_path, exist_ok=True)

        # interval_time = 2 * 60 * 60
        interval_time = timedelta(seconds=2 * 60 * 60)
        while start_time < end_time:
            if start_time + interval_time > end_time:
                current_et = end_time
            else:
                current_et = start_time + interval_time
            for metric in normal_metrics:
                data_raw = self.client.custom_query_range(
                    f"{metric}{{namespace='{self.namespace}'}}",
                    time_format_transform(start_time),
                    time_format_transform(current_et),
                    step=step,
                )
                # Debugging print statements
                # print(f"Query: {metric}{{namespace='{self.namespace}'}}")
                # print(f"Start Time: {start_time}, End Time: {current_et}")
                # print(f"Data Raw Length: {len(data_raw)}")
                # print(f"Data Raw: {data_raw}")
                if len(data_raw) == 0:
                    continue
                timestamp_list = []
                cmdb_id_list = []
                kpi_list = []
                value_list = []
                for data in data_raw:
                    if data["metric"]["pod"] not in self.pod_list:
                        continue
                    cmdb_id = data["metric"]["instance"] + "." + data["metric"]["pod"]
                    if cmdb_id == "":
                        continue
                    kpi_name = metric
                    if metric in network_metrics:
                        kpi_name = network_kpi_name_format(data["metric"])
                    for d in data["values"]:
                        timestamp_list.append(int(d[0]))
                        cmdb_id_list.append(cmdb_id)
                        kpi_list.append(kpi_name)
                        value_list.append(round(float(d[1]), 3))
                dt = pd.DataFrame(
                    {
                        "timestamp": timestamp_list,
                        "cmdb_id": cmdb_id_list,
                        "kpi_name": kpi_list,
                        "value": value_list,
                    }
                )
                dt = dt.sort_values(by="timestamp")
                file_path = os.path.join(container_save_path, "kpi_" + metric + ".csv")
                if os.path.exists(file_path):
                    with open(file_path, "a", encoding="utf-8", newline="") as f:
                        dt.to_csv(f, header=False, index=False)
                else:
                    dt.to_csv(file_path, index=False)

            # # for metric in istio_metrics:
            #     data_raw = self.client.custom_query_range(f"{metric}{{namespace='{namespace}'}}", time_format_transform(start_time), time_format_transform(current_et), step=step)
            #     if len(data_raw) == 0:
            #         continue
            #     timestamp_list = []
            #     cmdb_id_list = []
            #     kpi_list = []
            #     value_list = []
            #     for data in data_raw:
            #         cmdb_id = istio_cmdb_id_format(data['metric'])
            #         pod_name = cmdb_id.split('.')[0]
            #         if cmdb_id == '' or pod_name not in pod_list:
            #             continue
            #         kpi_name = istio_kpi_name_format(data['metric'])
            #         for d in data['values']:
            #             timestamp_list.append(int(d[0]))
            #             cmdb_id_list.append(cmdb_id)
            #             kpi_list.append(kpi_name)
            #             value_list.append(round(float(d[1]), 3))
            #     dt = pd.DataFrame({
            #         'timestamp': timestamp_list,
            #         'cmdb_id': cmdb_id_list,
            #         'kpi_name': kpi_list,
            #         'value': value_list
            #     })
            #     dt = dt.sort_values(by='timestamp')
            #     file_path = os.path.join(istio_save_path, 'kpi_'+metric+'.csv')
            #     if os.path.exists(file_path):
            #         with open(file_path, 'a', encoding='utf-8', newline='') as f:
            #             dt.to_csv(f, header=False, index=False)
            #     else:
            #         dt.to_csv(file_path, index=False)
            start_time = current_et
        return f"Metrics data exported to directory: {save_path}"

    def get_all_metrics(self):
        """Get all of the metrics"""
        all_metrics = self.client.all_metrics()
        all_metrics = list(
            filter(lambda x: True if x in normal_metrics else False, all_metrics)
        )
        return all_metrics


if __name__ == "__main__":
    prom = PrometheusAPI(monitor_config["prometheusApi"], monitor_config["namespace"])

    # Define time range for exporting metrics
    end_time = datetime.now()
    start_time = end_time - timedelta(minutes=7)

    # Define the save path for metrics
    save_path = root_path / "metrics_output"

    prom.export_all_metrics(
        start_time=start_time, end_time=end_time, save_path=str(save_path), step=10
    )
