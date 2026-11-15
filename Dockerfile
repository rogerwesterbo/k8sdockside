# syntax=docker/dockerfile:1

# ---- build: render the Slidev deck to static files ----
# Runs on the build host's native arch; the output is plain HTML/JS/CSS.
FROM --platform=$BUILDPLATFORM node:26-alpine AS builder
WORKDIR /app

# playwright-chromium is only needed for PDF export, not for `slidev build`
ENV PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# ---- runtime: serve dist/ with unprivileged nginx on :8080 ----
FROM nginxinc/nginx-unprivileged:stable-alpine
WORKDIR /app

COPY nginx/http.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist/ /app/

EXPOSE 8080
