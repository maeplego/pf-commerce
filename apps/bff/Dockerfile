FROM node:22-alpine
WORKDIR /app
COPY package.json ./
COPY src ./src
RUN npm install --omit=dev
EXPOSE 8110
ENV COMMERCE_HTTP_PORT=8110
CMD ["node", "src/server.js"]
