# Nginx-image til serveren med konfigurationen bygget ind
FROM nginx:1.31.7-alpine

COPY nginx.prod.conf /etc/nginx/nginx.conf
COPY security-headers.conf /etc/nginx/security-headers.conf

EXPOSE 80 443
