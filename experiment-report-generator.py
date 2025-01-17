# import csv
# from datetime import datetime, timezone, timedelta

# # Experiment data
# experiments = [
#     {
#         "name": "user_unregistered_mongodb-analysis-1",
#         "start": "2025-01-13 12:16:02.038039 PST",
#         "end": "2025-01-13 12:17:22.016247 PST",
#     },
#     {
#         "name": "user_unregistered_mongodb-mitigation-1",
#         "start": "2025-01-13 12:17:52.016468 PST",
#         "end": "2025-01-13 12:19:11.389775 PST",
#     },
#     {
#         "name": "user_unregistered_mongodb-detection-2",
#         "start": "2025-01-13 12:19:41.389936 PST",
#         "end": "2025-01-13 12:21:01.687834 PST",
#     },
#     {
#         "name": "user_unregistered_mongodb-localization-2",
#         "start": "2025-01-13 12:21:31.688006 PST",
#         "end": "2025-01-13 12:22:52.441489 PST",
#     },
#     {
#         "name": "user_unregistered_mongodb-analysis-2",
#         "start": "2025-01-13 12:23:22.441660 PST",
#         "end": "2025-01-13 12:24:43.353925 PST",
#     },
#     {
#         "name": "user_unregistered_mongodb-mitigation-2",
#         "start": "2025-01-13 12:25:13.354138 PST",
#         "end": "2025-01-13 12:26:33.557882 PST",
#     },
#     {
#         "name": "misconfig_app_hotel_res-detection-1",
#         "start": "2025-01-13 12:27:03.558021 PST",
#         "end": "2025-01-13 12:28:29.980629 PST",
#     },
#     {
#         "name": "misconfig_app_hotel_res-localization-1",
#         "start": "2025-01-13 12:28:59.981068 PST",
#         "end": "2025-01-13 12:30:26.151170 PST",
#     },
#     {
#         "name": "misconfig_app_hotel_res-analysis-1",
#         "start": "2025-01-13 12:30:56.151331 PST",
#         "end": "2025-01-13 12:32:24.154419 PST",
#     },
#     {
#         "name": "misconfig_app_hotel_res-mitigation-1",
#         "start": "2025-01-13 12:32:54.154566 PST",
#         "end": "2025-01-13 12:34:20.430193 PST",
#     },
#     {
#         "name": "pod_failure_hotel_res-detection-1",
#         "start": "2025-01-13 12:34:50.430332 PST",
#         "end": "2025-01-13 12:36:14.094302 PST",
#     },
#     {
#         "name": "pod_failure_hotel_res-localization-1",
#         "start": "2025-01-13 12:36:44.094474 PST",
#         "end": "2025-01-13 12:38:06.468597 PST",
#     },
#     {
#         "name": "network_loss_hotel_res-detection-1",
#         "start": "2025-01-13 12:38:36.468805 PST",
#         "end": "2025-01-13 12:39:59.021991 PST",
#     },
#     {
#         "name": "network_loss_hotel_res-localization-1",
#         "start": "2025-01-13 12:40:29.022145 PST",
#         "end": "2025-01-13 12:41:52.108492 PST",
#     },
#     {
#         "name": "noop_detection_hotel_reservation-1",
#         "start": "2025-01-13 12:42:22.108649 PST",
#         "end": "2025-01-13 12:43:37.086271 PST",
#     },
# ]

# # Helper to convert PST time to UTC epoch milliseconds, rounding as needed
# def pst_to_epoch_rounded(pst_time, round_up=False):
#     pst = timezone(timedelta(hours=-8))  # PST offset
#     dt = datetime.strptime(pst_time, "%Y-%m-%d %H:%M:%S.%f PST").replace(tzinfo=pst)
#     if round_up:
#         # Round up to the nearest minute
#         dt = (dt + timedelta(minutes=1)).replace(second=0, microsecond=0) if dt.second > 0 or dt.microsecond > 0 else dt.replace(second=0, microsecond=0)
#     else:
#         # Round down to the nearest minute
#         dt = dt.replace(second=0, microsecond=0)
#     return int(dt.timestamp() * 1000)

# # Generate URLs and write to CSV
# results_filename = "experiment_urls"
# results_filename_extension = ".csv"
# log_filename = results_filename+results_filename_extension
# base_url = (
#     "https://app.datadoghq.com/apm/entity/service%3Afrontend?compareVersionEnd=0"
#     "&compareVersionPaused=false&compareVersionStart=0&dependencyMap=qson%3A%28data%3A%28telemetrySelection%3Aall_sources%29%2Cversion%3A%210%29"
#     "&deployments=qson%3A%28data%3A%28hits%3A%28selected%3Aversion_count%29%2Cerrors%3A%28selected%3Aversion_count%29"
#     "%2Clatency%3A%28selected%3Ap95%29%2CtopN%3A%215%29%2Cversion%3A%210%29&env=aiopslab&fromUser=true"
#     "&groupMapByOperation=null&infrastructure=qson%3A%28data%3A%28viewType%3Apods%29%2Cversion%3A%210%29"
#     "&operationName=http.request&panels=qson%3A%28data%3A%28%29%2Cversion%3A%210%29&resources=qson%3A%28data%3A%28visible%3A%21t%2Chits%3A%28selected%3Atotal%29"
#     "%2Cerrors%3A%28selected%3Atotal%29%2Clatency%3A%28selected%3Ap95%29%2CtopN%3A%215%29%2Cversion%3A%211%29&summary=qson%3A%28data%3A%28visible%3A%21t"
#     "%2Cchanges%3A%28%29%2Cerrors%3A%28selected%3Acount%29%2Chits%3A%28selected%3Acount%29%2Clatency%3A%28selected%3Alatency%2Cslot%3A%28agg%3A95%29"
#     "%2Cdistribution%3A%28isLogScale%3A%21f%29%2CshowTraceOutliers%3A%21t%29%2Csublayer%3A%28selected%3Apercentage%2Cslot%3A%28layers%3Aservice%29%29"
#     "%2ClagMetrics%3A%28selectedMetric%3A%21s%2CselectedGroupBy%3A%21s%29%29%2Cversion%3A%211%29&paused=true"
# )

# with open(log_filename, mode="w", newline="") as file:
#     writer = csv.writer(file)
#     writer.writerow(["Experiment Name", "Start Time (PST)", "End Time (PST)", "Generated URL"])

#     for experiment in experiments:
#         start_epoch = pst_to_epoch_rounded(experiment["start"], round_up=False)
#         end_epoch = pst_to_epoch_rounded(experiment["end"], round_up=True)
#         url = f"{base_url}&start={start_epoch}&end={end_epoch}"
#         writer.writerow([experiment["name"], experiment["start"], experiment["end"], url])

# timestamp = datetime.now(PACIFIC_TIMEZONE).strftime("%Y%m%d%H%M%S")
# new_log_filename = f"{results_filename}_{timestamp}{results_filename_extension}"
# os.rename(log_filename, new_log_filename)
# print(f"Log file renamed to {new_log_filename}.")

# print(f"URLs have been generated and saved to {csv_filename}.")

import csv
import os
import sys
from datetime import datetime, timezone, timedelta

# Helper to convert PST time to UTC epoch milliseconds, rounding as needed
def pst_to_epoch_rounded(pst_time, round_up=False):
    try:
        pst = timezone(timedelta(hours=-8))  # PST offset
        # Ensure the string is clean
        pst_time = pst_time.strip()
        dt = datetime.strptime(pst_time, "%Y-%m-%d %H:%M:%S PST").replace(tzinfo=pst)
        if round_up:
            # Round up to the nearest minute
            dt = (dt + timedelta(minutes=1)).replace(second=0, microsecond=0) if dt.second > 0 or dt.microsecond > 0 else dt.replace(second=0, microsecond=0)
        else:
            # Round down to the nearest minute
            dt = dt.replace(second=0, microsecond=0)
        return int(dt.timestamp() * 1000)
    except ValueError as e:
        print(f"Error parsing time: '{pst_time}'. Ensure it matches the format '%Y-%m-%d %H:%M:%S PST'.")
        raise e

def main(input_file):
    results_filename = "experiment_urls"
    results_filename_extension = ".csv"
    log_filename = results_filename + results_filename_extension

    base_url = (
        "https://app.datadoghq.com/apm/entity/service%3Afrontend?compareVersionEnd=0"
        "&compareVersionPaused=false&compareVersionStart=0&dependencyMap=qson%3A%28data%3A%28telemetrySelection%3Aall_sources%29%2Cversion%3A%210%29"
        "&deployments=qson%3A%28data%3A%28hits%3A%28selected%3Aversion_count%29%2Cerrors%3A%28selected%3Aversion_count%29"
        "%2Clatency%3A%28selected%3Ap95%29%2CtopN%3A%215%29%2Cversion%3A%210%29&env=aiopslab&fromUser=true"
        "&groupMapByOperation=null&infrastructure=qson%3A%28data%3A%28viewType%3Apods%29%2Cversion%3A%210%29"
        "&operationName=http.request&panels=qson%3A%28data%3A%28%29%2Cversion%3A%210%29&resources=qson%3A%28data%3A%28visible%3A%21t%2Chits%3A%28selected%3Atotal%29"
        "%2Cerrors%3A%28selected%3Atotal%29%2Clatency%3A%28selected%3Ap95%29%2CtopN%3A%215%29%2Cversion%3A%211%29&summary=qson%3A%28data%3A%28visible%3A%21t"
        "%2Cchanges%3A%28%29%2Cerrors%3A%28selected%3Acount%29%2Chits%3A%28selected%3Acount%29%2Clatency%3A%28selected%3Alatency%2Cslot%3A%28agg%3A95%29"
        "%2Cdistribution%3A%28isLogScale%3A%21f%29%2CshowTraceOutliers%3A%21t%29%2Csublayer%3A%28selected%3Apercentage%2Cslot%3A%28layers%3Aservice%29%29"
        "%2ClagMetrics%3A%28selectedMetric%3A%21s%2CselectedGroupBy%3A%21s%29%29%2Cversion%3A%211%29&paused=true"
    )

    # Read the file and organize data
    pid_data = {}
    with open(input_file, "r") as file:
        for line in file:
            if line.strip():
                parts = line.split(", ")
                pid = parts[0].split(": ")[1]
                timestamp_type = parts[1].split(": ")[0]
                timestamp_value = parts[1].split(": ")[1]

                if pid not in pid_data:
                    pid_data[pid] = {}
                pid_data[pid][timestamp_type] = timestamp_value

    # Write results to CSV
    with open(log_filename, mode="w", newline="") as file:
        writer = csv.writer(file)
        writer.writerow(["Experiment Name", "Start Time (PST)", "End Time (PST)", "Generated URL"])

        for pid, timestamps in pid_data.items():
            if "Start Time" in timestamps and "End Time" in timestamps:
                start_epoch = pst_to_epoch_rounded(timestamps["Start Time"], round_up=False)
                end_epoch = pst_to_epoch_rounded(timestamps["End Time"], round_up=True)
                url = f"{base_url}&start={start_epoch}&end={end_epoch}"
                writer.writerow([pid, timestamps["Start Time"], timestamps["End Time"], url])

    # Rename the CSV with a timestamp
    timestamp = datetime.now(timezone.utc).strftime("%Y%m%d%H%M%S")
    new_log_filename = f"{results_filename}_{timestamp}{results_filename_extension}"
    os.rename(log_filename, new_log_filename)
    print(f"Log file renamed to {new_log_filename}.")

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python script.py <input_file>")
        sys.exit(1)

    input_file = sys.argv[1]

    if not os.path.isfile(input_file):
        print(f"Error: File '{input_file}' does not exist.")
        sys.exit(1)

    main(input_file)
