# Baked in rather than bind-mounted: a changed template then changes the image,
# so `docker compose up` recreates the gateway instead of keeping the old config.
FROM nginx:1.29-alpine
COPY default.conf.template /etc/nginx/templates/default.conf.template
