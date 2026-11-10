# Nginx-image til serveren med konfigurationen bygget ind
FROM nginx:1.30.6-alpine

# Henter sikkerhedsrettelser fra Alpine, som endnu ikke er med i det officielle nginx-image
RUN apk upgrade --no-cache

COPY nginx.prod.conf /etc/nginx/nginx.conf
COPY security-headers.conf /etc/nginx/security-headers.conf

EXPOSE 80 443
