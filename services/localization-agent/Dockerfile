FROM python:3.12-slim
WORKDIR /app
COPY --from=ghcr.io/astral-sh/uv:0.13.4 /uv /usr/local/bin/uv
COPY pyproject.toml uv.lock ./
COPY src ./src
COPY config ./config
RUN uv sync --frozen --no-dev && useradd --uid 10001 --create-home localizer && mkdir /state && chown localizer /state
ENV PATH="/app/.venv/bin:$PATH" LOCALIZATION_ROOT=/app LOCALIZATION_STATE_DIR=/state PYTHONDONTWRITEBYTECODE=1
USER 10001
EXPOSE 8000
CMD ["l10n", "serve"]
