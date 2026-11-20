# Leksuu 🌍
### An Open-Source, AI-Powered Multilingual Language Learning Platform

**Making quality language education more accessible, one language at a time.**

Leksuu is an open-source multilingual language-learning platform designed to make effective language education accessible to more people, especially speakers of underrepresented and low-resource languages.

Our initial focus is on **English, Persian (Farsi), and Central Kurdish (Sorani)**, with an architecture designed to support additional languages and learning communities.

Unlike conventional language-learning applications, Leksuu aims to combine proven language acquisition methodologies, intelligent learning tools, speech technologies, and community-driven education in one extensible platform.

---

## 🌱 Our Mission

Millions of people speak languages that receive limited attention from mainstream educational technology.

Many language-learning platforms focus on widely spoken languages, leaving smaller linguistic communities with fewer educational resources and less access to modern learning technologies.

Leksuu aims to address this gap by building an open-source foundation for multilingual education.

Our mission is to:

- Make high-quality language education accessible to underserved linguistic communities.
- Support languages such as Persian and Central Kurdish (Sorani).
- Use AI to improve language learning, pronunciation, translation, and educational content creation.
- Provide an extensible platform that developers can adapt and contribute to.
- Build an educational ecosystem that is not limited to a single language or geographical region.

## ✨ Key Features

### 1. Multilingual Architecture

- English, Persian, and Central Kurdish (Sorani) support.
- Localized application messages.
- Extensible language and course structures.
- Right-to-left (RTL) language support in the planned frontend design system.

### 2. Structured Language Learning

- CEFR-based course organization.
- Courses, lessons, and interactive exercises.
- Multiple exercise types:
  - Multiple choice
  - Fill in the blanks
  - Sentence reordering
  - Matching exercises
- Server-side answer validation.
- Learning progress tracking and feedback.

### 3. Intelligent Learning & Gamification

- XP and achievement system.
- Daily learning goals.
- Learning streaks.
- League-based progression.
- Spaced repetition system (SRS) foundations.

### 4. Speech & Pronunciation Learning

Leksuu's data architecture includes foundations for advanced pronunciation learning:

- Phoneme-level pronunciation scoring.
- Individual learner phoneme profiles.
- Targeted pronunciation drills.
- Mouth-shape guidance for phonemes.
- Speaking sessions and pronunciation progress tracking.

### 5. Stories & Immersive Learning

Inspired by language acquisition methodologies, including AJ Hoge's Effortless English approach.

- Mini-stories and interactive questions.
- Point-of-view story variations.
- Listen-and-answer learning.
- Deep listening and repetition tracking.
- Vocabulary learning through phrases and contextual expressions.

### 6. Community-Based Learning

Inspired by the collaborative learning approach used by platforms such as Busuu.

- Writing and speaking submissions.
- Community feedback.
- Native-speaker correction foundations.
- CEFR-oriented learning content.

### 7. AI-Assisted Media Processing

Leksuu includes an automated media preparation pipeline designed to support educational content creation.

Current capabilities and infrastructure include:

- Speech recognition using Whisper models.
- Audio-text alignment.
- Independent speech-recognition verification.
- Persian and Sorani translation workflows.
- Automated quality checks and exception detection.
- Background processing with retries and checkpoints.
- Human review before publication.

The system is designed to improve the efficiency and consistency of multilingual educational content preparation.

AI-generated content remains subject to quality checks and editorial approval.

---

## 🤖 Why AI Matters for Leksuu

Artificial intelligence is not simply an optional feature in Leksuu. It is an important part of our long-term vision.

We want to make advanced language-learning capabilities available to communities that are often overlooked by mainstream educational platforms.

Our AI development roadmap includes:

| Area | Objective |
|---|---|
| Speech Recognition | Accurate speech-to-text processing |
| Pronunciation Assessment | Evaluating learner pronunciation |
| Phoneme Analysis | Identifying individual pronunciation difficulties |
| Translation | English, Persian, and Sorani translation workflows |
| Educational Content | Creating and improving multilingual learning materials |
| Conversational Learning | Developing interactive AI-assisted learning experiences |
| Content Quality | Evaluating translations, transcripts, and educational accuracy |

We are particularly interested in improving AI-assisted learning for Central Kurdish (Sorani), where access to advanced language-learning technology remains limited.

Our goal is to build these capabilities with an emphasis on quality, transparency, and accessibility.

---

## 🏗️ Technology Stack

Leksuu uses a modular architecture designed for maintainability, scalability, and community contributions.

| Component | Technology |
|---|---|
| Backend | Go 1.26 |
| Architecture | Modular Monolith / Clean Architecture |
| Database | PostgreSQL 16 |
| Caching & Infrastructure | Redis |
| Authentication | JWT + Refresh Token Rotation |
| Authorization | Role-Based Access Control (RBAC) |
| Database Migrations | golang-migrate |
| Media Processing | Python 3.11, Whisper, PyAV |
| Translation Infrastructure | Local MADLAD models |
| Deployment | Docker & Docker Compose |
| Object Storage | Local Storage / S3-Compatible Storage |
| Frontend | Next.js (planned design system) |

### Architecture Principles

- Clean Architecture
- Domain-driven modular boundaries
- Dependency inversion
- Interface-based repositories
- Testable business logic
- Build-time infrastructure selection
- Environment-based configuration
- Self-hosting support

---

## 📊 Development Status

Leksuu is under active development.

The current development phase focuses on building a reliable backend foundation before expanding the complete learning experience.

| Component | Status |
|---|---|
| Modular Clean Architecture | ✅ Implemented |
| Multilingual Message Catalog | ✅ Implemented |
| PostgreSQL Schema & Migrations | ✅ Verified |
| Authentication & JWT | ✅ Implemented and tested |
| Refresh Token Rotation | ✅ Implemented and tested |
| RBAC | ✅ Implemented |
| User Profile Management | ✅ Implemented |
| Course & Exercise Management | ✅ Implemented |
| Server-Side Exercise Grading | ✅ Implemented |
| Gamification Foundation | ✅ Implemented |
| Admin Authoring | ✅ Implemented |
| Media Storage Abstraction | ✅ Implemented |
| Automated Media Processing | ✅ Implemented |
| AI-Assisted Content Workflows | 🚧 Under development |
| Frontend Design System | 🚧 Planned |
| Complete Learning Experience | 🚧 Roadmap |

---

## 🚀 Getting Started

### Prerequisites

- Go 1.26
- Docker and Docker Compose
- PostgreSQL 16 (if running without Docker)

### Run with Docker Compose

Clone the repository:

```bash
git clone https://github.com/YOUR_USERNAME/leksuu.git
cd leksuu/backend
```

Start the backend infrastructure:

```bash
docker compose up --build
```

This starts:

- Go API server
- PostgreSQL
- Redis
- Database migration process

Check the API health:

```bash
curl http://localhost:8080/healthz
```

### Run Without PostgreSQL

The default development build supports in-memory repositories:

```bash
go run ./cmd/api
```

For PostgreSQL-backed execution:

```bash
go mod tidy
go run -tags pgx ./cmd/migrate
go run -tags pgx ./cmd/api
```

For more details, see the backend documentation.

---

## 🧠 Learning Methodologies

Leksuu combines ideas from several established approaches to language acquisition.

| Methodology | Implementation in Leksuu |
|---|---|
| AJ Hoge – Effortless English | Mini-stories, point-of-view variations, listen-and-answer |
| ELSA-style Pronunciation | Phoneme-level scoring and targeted pronunciation drills |
| Busuu | Structured CEFR courses and community feedback |
| Leitner System | Spaced repetition foundations |
| Gamified Learning | XP, streaks, achievements, and daily goals |

These approaches provide the pedagogical foundation for building a more engaging and effective learning experience.

---

## 🌍 Open Source & Community Impact

Leksuu is being developed with extensibility and community participation in mind.

We believe that language technology should not be restricted to languages with large commercial markets.

By developing reusable infrastructure for multilingual learning, we hope to contribute to:

- More inclusive educational technology.
- Better support for underrepresented languages.
- Open-source speech and language-learning tools.
- Reusable multilingual learning infrastructure.
- Collaboration between developers, educators, and language communities.

We welcome developers, educators, translators, and language enthusiasts who share this vision.

---

## 🗺️ Roadmap

### Phase 0 — Backend Foundation
- [x] Modular backend architecture
- [x] Authentication and authorization
- [x] Database schema and migrations
- [x] Course and exercise foundations
- [x] Gamification foundations
- [x] Media storage infrastructure

### Phase 1 — Core Learning Experience
- [ ] Frontend design system
- [ ] Course and lesson player
- [ ] Vocabulary and SRS
- [ ] Learning progress dashboard
- [ ] Interactive learning experience

### Phase 2 — AI-Powered Learning
- [ ] Advanced pronunciation assessment
- [ ] AI-assisted speaking exercises
- [ ] Improved multilingual translation
- [ ] Intelligent learning feedback
- [ ] AI-assisted content generation

### Phase 3 — Community & Expansion
- [ ] Native-speaker learning community
- [ ] More supported languages
- [ ] Expanded educational content
- [ ] Offline-first learning experience
- [ ] Developer contribution ecosystem

---

## 🤝 Contributing

We believe the future of multilingual education should be built collaboratively.

Contributions are welcome in areas such as:

- Go backend development
- Frontend development
- AI and machine learning
- Speech recognition and pronunciation assessment
- Persian and Kurdish language resources
- Translation and educational content
- Testing, documentation, and architecture

If you are interested in contributing, feel free to open an issue or submit a pull request.

---

## 💡 Support the Project

Building an open-source multilingual learning platform requires continuous development, experimentation, and infrastructure investment.

Support from the open-source and AI communities can help accelerate:

- AI-powered language-learning research.
- Speech and pronunciation technologies.
- Low-resource language support.
- Better educational tools for underserved communities.

Our long-term goal is to make Leksuu a reusable open-source foundation for accessible multilingual education.

---

## 📜 License

See the repository's LICENSE file for licensing information.

---

**Leksuu — Language learning without linguistic boundaries.** 🌍