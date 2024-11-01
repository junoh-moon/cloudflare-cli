FROM gradle:jdk11 AS builder

ENV HOME=/root
ENV TZ=Asia/Seoul

RUN mkdir -p /app/
WORKDIR /app/
COPY . /app/
RUN ./gradlew shadowJar

FROM eclipse-temurin:17-jre-alpine
COPY --from=builder /app/build/libs/cloudflare-sdk-1.0-all.jar ./

ENTRYPOINT ["java", "-jar", "cloudflare-sdk-1.0-all.jar"]
