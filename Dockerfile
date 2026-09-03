# ---- build ----------------------------------------------------------------
FROM eclipse-temurin:21-jdk AS build
WORKDIR /build

# Warm the dependency cache before copying sources.
COPY .mvn/ .mvn/
COPY mvnw pom.xml ./
RUN ./mvnw -B -ntp dependency:go-offline

COPY src/ src/
RUN ./mvnw -B -ntp -DskipTests package && cp target/*.jar app.jar

# ---- run ------------------------------------------------------------------
FROM eclipse-temurin:21-jre-alpine
WORKDIR /app

RUN addgroup -S keepalive && adduser -S -G keepalive keepalive
COPY --from=build /build/app.jar app.jar
USER keepalive

EXPOSE 8088
ENTRYPOINT ["java", "-XX:MaxRAMPercentage=75", "-jar", "/app/app.jar"]
