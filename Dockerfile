FROM gradle:jdk11 AS builder

ENV HOME=/root

WORKDIR /root/
COPY . /root/
RUN ./gradlew shadowJar

FROM eclipse-temurin:17-jre-alpine
COPY --from=builder /root/build/libs/cloudflare-sdk-1.0-all.jar ./

ENTRYPOINT ["java", "-jar", "cloudflare-sdk-1.0-all.jar"]
