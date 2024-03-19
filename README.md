# HTTP Downloader

## Overview

This project is a simple HTTP downloader written entirely in Go, utilizing system calls (syscalls) for network communication. It is designed to download files from a specified server, with the ability to choose between single or multiple connections for the download process. The choice between single and multiple connections is determined by the file's size and the server's support for the `Range` request header.

## Example Usage

**Prepare the Server**: Copy the files you wish to download into the `/tmp/nginx_data` directory. This directory is mounted into the Nginx container, making the files available for download.

```bash
docker-compose up
docker attach http_downloader
./http_dl 10.11.0.10 80 filename
```
