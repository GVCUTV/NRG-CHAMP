# Aggregator Service Documentation

This document presents only the **Aggregator Service** of the NRG CHAMP platform. It consolidates all the details available in the project documentation, focusing solely on its scope.

## Functional Requirements (Aggregator Scope)

### 2.1.1. Data Collection and Environmental Monitoring

* **Sensor Integration:**
    * Support for a wide range of IoT sensors to monitor temperature, humidity, energy consumption, and other relevant environmental parameters (e.g., CO₂ levels, occupancy).
    * Real-time data acquisition from indoor and outdoor sensor networks.
* **Data Aggregation:**
    * Collection and aggregation of sensor data for further analysis.
    * Support for data buffering and recovery in case of temporary network failures.


## Use Case: Data Collection and Aggregation

## 4.1. Use Case: Data Collection and Environmental Monitoring

**Actors:**

* IoT Sensors
* Data Aggregator Service
* Building Management System (BMS)

**Description:**  
This use case describes the process of continuously acquiring and aggregating environmental data (e.g., temperature, humidity) from a distributed network of IoT sensors installed throughout a building.

**Preconditions:**

* IoT sensors are installed and configured.
* A reliable network connection exists between sensors and the central Data Aggregator Service.

**Basic Flow:**

1. **Sensor Activation:** IoT sensors are activated and begin collecting environmental data continuously.
2. **Data Transmission:** Each sensor transmits real-time measurements (temperature, humidity, etc.) to the Data Aggregator Service.
3. **Data Aggregation:** The Data Aggregator Service receives, timestamps, and consolidates the data from all sensors.
4. **Data Storage:** Aggregated data is temporarily stored in a local cache or directly forwarded to the central database for processing by other modules.

**Alternative Flows:**

* **AF1 – Network Interruption:**
    * **Step 2A:** If the sensor loses network connectivity, it buffers data locally.
    * **Step 2B:** Once the connection is re-established, the buffered data is transmitted to the aggregator.
* **AF2 – Sensor Failure:**
    * **Step 1A:** If a sensor malfunctions, the BMS detects the failure and generates an alert for maintenance.

**Extension Points:**

* **EP1:** Data Validation – Before storage, data can be validated for accuracy and consistency.
* **EP2:** Pre-Processing – Data can be pre-processed (e.g., filtered or aggregated) if required by the Analytics Engine.

**Inclusions:**

* **I1:** Operations Logging – In all flows, operations are logged and reported to the system monitoring service.

---


## Architecture: Aggregator and Pre-Processing Module

* **Data Aggregator and Pre-Processing Module:**

## Core Module: Data Aggregator

### 6.1.2. Data Aggregator

* **Batching & Windowing:**

    * Group incoming messages into fixed-length time windows (e.g. 1 minute) for downstream consumption.

* **Pre‑Processing:**

    * Outlier detection (e.g. spikes/drops) and simple smoothing filters.

* **Fan‑out:**

    * Publish cleaned batches to:

        1. Time‑Series Database (InfluxDB/Prometheus)

        2. Internal Message Queue (Kafka/RabbitMQ) for MAPE & Blockchain modules

* **Health Checks:**

    * HTTP health endpoint, self‑test on startup (DB connectivity, queue reachability).

---


## Blockchain Integration (Aggregator Role)

1. **Data Aggregator → Transaction Builder**

* **Responsibility:** Receives cleaned, batched sensor readings and MAPE control events.

* **Interface:** In‐memory queue (Kafka/RabbitMQ).