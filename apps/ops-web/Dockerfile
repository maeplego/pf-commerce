FROM node:22-alpine AS build
WORKDIR /app
COPY package.json ./
RUN npm install --omit=dev
COPY . .
ARG NEXT_PUBLIC_COMMERCE_API_URL=http://localhost:8099
ENV NEXT_PUBLIC_COMMERCE_API_URL=$NEXT_PUBLIC_COMMERCE_API_URL
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

FROM node:22-alpine AS run
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=build /app/public ./public
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
EXPOSE 3010
ENV PORT=3010
CMD ["node", "server.js"]
