# The static build output does not depend on the target architecture, so it is
# built once on the build machine, without emulation, for every target.
FROM --platform=$BUILDPLATFORM node:24-bookworm-slim@sha256:dc26ae92c9fb15cf6b1d63e20f0944b2efe0f115b20e00be48fb23fceb061c54 AS web
WORKDIR /src
COPY package*.json ./
RUN npm ci --no-audit --no-fund
COPY . ./
RUN npm run check && npm run build

FROM nginx:1.30.7-alpine@sha256:caf4e1e032a20f9c3c5e28fc7f25ffd768ad8677b4975c54193075997b3a97e3
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=web /src/build/ /usr/share/nginx/html/
