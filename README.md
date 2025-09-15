# 🚀 Go Uptime Monitor

<div align="center">

### Production-Grade High-Performance Service Status Monitoring System

*A lightweight, high-concurrency service health monitoring solution built with Go + Redis*

[![🌐 Live Demo](https://img.shields.io/badge/🌐_Live_Demo-Online-brightgreen?style=for-the-badge)](https://status.435534.xyz)
[![🐳 Docker](https://img.shields.io/badge/🐳_Docker-Ready-blue?style=for-the-badge&logo=docker&logoColor=white)](https://hub.docker.com/)
[![⚡ Go](https://img.shields.io/badge/⚡_Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org/)
[![📦 Redis](https://img.shields.io/badge/📦_Redis-7+-DC382D?style=for-the-badge&logo=redis&logoColor=white)](https://redis.io/)

---

*Currently compiled and running on Google E2-Micro instance*

</div>

## ✨ Project Highlights

Go Uptime Monitor is not just a fully functional application, but also a comprehensive example showcasing **backend concurrent programming**, **robust system design**, **frontend performance optimization**, and **production-grade containerization practices**.

### 🎯 Core Features

<table>
<tr>
<td width="33%" valign="top">

#### 🚀 High-Performance Backend (Go)
- **High-concurrency Worker Pool** model, easily scalable to monitor hundreds of services
- **Graceful Shutdown** mechanism ensures smooth service termination without data loss
- **Robust Redis integration** with connection pooling, retry logic, and timeout control
- Detailed `health` and `metrics` API endpoints

</td>
<td width="33%" valign="top">

#### ⚡ Lightweight Frontend (Vanilla JS)
- **Zero frontend dependencies**, no npm or build tools required
- **Virtual DOM concept** with batch UI updates for optimized rendering
- **Client-side caching** + request timeout control
- **Page Visibility API** for intelligent resource conservation

</td>
<td width="33%" valign="top">

#### 📦 Production-Grade Containerization (Docker)
- **Multi-stage build** Dockerfile for small and secure images
- **Non-root user** execution following security best practices
- **Health checks** ensure proper service startup order
- One-click deployment with `docker-compose.yml`

</td>
</tr>
</table>

### 💾 Efficient Data Handling (Redis)

- 📊 **Redis Streams** for storing high-frequency raw monitoring data
- 📈 **Redis Hashes** for storing daily aggregated statistics, improving query performance  
- 🧹 **Automatic data cleanup strategy** to prevent unlimited Redis memory growth

---

## 🛠️ Tech Stack

<div align="center">

| Backend | Storage | Frontend | Containerization |
|:---:|:---:|:---:|:---:|
| ![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white) | ![Redis](https://img.shields.io/badge/Redis-7-DC382D?logo=redis&logoColor=white) | ![JavaScript](https://img.shields.io/badge/JavaScript-ES6+-F7DF1E?logo=javascript&logoColor=black) | ![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white) |

</div>

---

## 🚀 Quick Start

> 💡 **We highly recommend using Docker Compose** to start the project - it's the simplest and most reliable method

### 📦 Using Docker Compose (Recommended)

```bash
# 1. Clone the project
git clone https://github.com/solingerz/go-uptime-monitor.git
cd go-uptime-monitor

# 2. Create configuration file
cp .env.example .env

# 3. Start services
sudo docker compose up -d --build

# 4. Access monitoring dashboard
# Open your browser and visit http://localhost:8080
```

> ✅ The `app` service will intelligently wait for the `redis` service to be fully ready before starting

### 🔧 Local Development

If you prefer running locally, ensure you have Go (1.22+) and Redis installed.

```bash
# 1. Start Redis service
redis-server

# 2. Install dependencies
go mod download

# 3. Run the application
go run main.go
```

---

## ⚙️ Configuration

### 🎯 Monitoring Target Configuration

Two configuration methods are supported, with **environment variables having higher priority**:

<details>
<summary><b>Method 1: Environment Variables (Recommended)</b></summary>

Set `UPTIME_TARGETS_JSON` in your `.env` file:

```env
UPTIME_TARGETS_JSON='[
  {"Name":"Google","URL":"https://google.com"},
  {"Name":"GitHub","URL":"https://github.com"},
  {"Name":"Stack Overflow","URL":"https://stackoverflow.com"}
]'
```

</details>

<details>
<summary><b>Method 2: Configuration File</b></summary>

Create `config.json` in the project root:

```json
[
  {
    "Name": "Google",
    "URL": "https://www.google.com"
  },
  {
    "Name": "GitHub", 
    "URL": "https://github.com"
  }
]
```

</details>

### 🔧 Environment Variables Explained

<table>
<thead>
<tr>
<th>Variable Name</th>
<th>Default Value</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td><code>PORT</code></td>
<td><code>8080</code></td>
<td>HTTP port the application listens on</td>
</tr>
<tr>
<td><code>REDIS_ADDR</code></td>
<td><code>localhost:6379</code></td>
<td>Redis server address (should be <code>redis:6379</code> in Docker Compose)</td>
</tr>
<tr>
<td><code>REDIS_PASSWORD</code></td>
<td><em>(empty)</em></td>
<td>Redis password</td>
</tr>
<tr>
<td><code>REDIS_DB</code></td>
<td><code>0</code></td>
<td>Redis database number</td>
</tr>
<tr>
<td><code>WORKER_COUNT</code></td>
<td><code>5</code></td>
<td>Number of worker goroutines for concurrent checks</td>
</tr>
<tr>
<td><code>CHECK_INTERVAL_SECONDS</code></td>
<td><code>60</code></td>
<td>Interval in seconds for each health check round</td>
</tr>
<tr>
<td><code>AGG_INTERVAL_MINUTES</code></td>
<td><code>5</code></td>
<td>Interval in minutes for the data aggregation task</td>
</tr>
<tr>
<td><code>REQUEST_TIMEOUT_SECONDS</code></td>
<td><code>15</code></td>
<td>HTTP request timeout in seconds when checking target URLs</td>
</tr>
<tr>
<td><code>UPTIME_TARGETS_JSON</code></td>
<td><em>(empty)</em></td>
<td>JSON string defining the list of monitoring targets</td>
</tr>
</tbody>
</table>

---

## 📊 API Endpoints

The project provides a complete set of RESTful APIs to retrieve monitoring data:

<div align="center">

| Endpoint | Method | Description | Example |
|:---:|:---:|:---|:---|
| `/api/services` | GET | Get the list of all monitored services | `GET /api/services` |
| `/api/status` | GET | Get status data for services | `GET /api/status?window=24h` |
| `/api/health` | GET | Check application health status | `GET /api/health` |
| `/api/metrics` | GET | Get application metrics data | `GET /api/metrics` |

</div>

### 🔍 Query Parameters

- **`window`** parameter supports:
  - Hourly: `1h`, `2h`, `24h`
  - Daily: `1d`, `7d`, `30d`
  - Default: `1h`

---

## 📁 Project Structure

```
📦 go-uptime-monitor/
├── 🚀 main.go              # Main Go application
├── ⚙️  config.json          # Monitoring target configuration file (fallback)
├── 📁 static/
│   └── 🌐 index.html       # Frontend single-page application (HTML + CSS + JS)
├── 📦 go.mod               # Go module dependency file
├── 🔒 go.sum               # Go module checksums
├── 🐳 Dockerfile           # Dockerfile for building production image
├── 🐙 docker-compose.yml   # Docker Compose configuration
├── 📄 .env.example         # Example environment variables file
└── 📖 README.md            # Project documentation
```

---

## 🤝 Contributing

<div align="center">

Contributions of any kind are welcome! If you have a great idea or find a bug, please feel free to submit an Issue or Pull Request.

[![Contributors](https://img.shields.io/badge/Contributors-Welcome-brightgreen?style=for-the-badge&logo=github)](https://github.com/solingerz/go-uptime-monitor/issues)

</div>

### Contributing Steps

1. 🍴 Fork this repository
2. 🌿 Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. 💡 Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. 🚀 Push to the branch (`git push origin feature/AmazingFeature`)
5. 🎉 Open a Pull Request

---

## 📝 License

<div align="center">

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge)](https://opensource.org/licenses/MIT)

This project is licensed under the [MIT License](LICENSE).

---

### ⭐ If this project helped you, please give it a star!

[![GitHub stars](https://img.shields.io/github/stars/solingerz/go-uptime-monitor?style=social)](https://github.com/solingerz/go-uptime-monitor/stargazers)

</div>