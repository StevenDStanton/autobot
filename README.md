# autobotgo

Automatic LLM Generated Stories, PodCasts, and Videos in Go

## To Do

- [x] Setup http server to listen for external cron trigger
- [ ] Setup Auto Deploy Workflow
- [ ] Write .env from workflow
- [ ] Set up SQLite
- [ ] Setup Turso DB
- [ ] Setup Amazon S3 Saves
- [ ] Select Random Genre
- [ ] Generate Story - Initially use Open AI
- [ ] Save Story to DB
- [ ] Generate images - Initially Use OpenAI
- [ ] save images to s3
- [ ] Resize Images
- [ ] Convert images to WebP
- [ ] generate Audio - Initially use Open AI
- [ ] Save audio to s3
- [ ] Generate Video with FFMpeg (??)
- [ ] Save Video to S3
- [ ] Write Blog
- [ ] Upload Pod Cast
- [ ] Upload Video to Youtube
- [ ] Expand additional AI offerings
- [ ] Expand to Digital Ocean Saves
- [ ] Expand to Linode Saves
- [ ] Expand DB to include DynamoDB
- [ ] Deploy own LLM for Story Generation
- [ ] Deploy own Image Generation
- [ ] Deploy own TTS for Audio Generation
- [ ] Look into other video generation methods

### Auto Story Generator Design

### Domain

```mermaid
graph TD
    A[/writersgrove-net/auto/]
    A --> B[/src/]
    B --> C[/domain/]
    C --> D[/entities/]
    D --> E[Story.ts]
    C --> F[/value-objects/]
    F --> G[Genre.ts]
    C --> H[/aggregates/]
    H --> I[StoryAggregate.ts]
    C --> J[/events/]
    J --> K[StoryGenerated.ts]
    C --> L[/repositories/]
    L --> M[StoryRepository.ts]

```

### Commands

```mermaid
graph TD
    B[/src/]
    B --> N[/application/]
    N --> O[/commands/]
    O --> P[GenerateStoryCommand.ts]
    O --> Q[ConvertStoryToAudioCommand.ts]
    O --> R[GenerateStoryImagesCommand.ts]
```

### Infrastructure

```mermaid
graph TD
    B[/src/]
    B --> S[/infrastructure/]
    S --> T[/repositories/]
    T --> U[HugoStoryRepository.ts]
    S --> V[/services/]
    V --> W[OpenAIServiceImpl.ts]
    V --> X[AudioServiceImpl.ts]
    V --> Y[ImageServiceImpl.ts]

```

### Tests and other files

```mermaid
graph TD
    A[/writersgrove-net/auto/]
    A --> B[/src/]
    B --> BA[index.ts]
    A --> AC[/test/]
    AC --> AD[domain/]
    AC --> AE[application/]
    AC --> AF[infrastructure/]
    AC --> AG[/mocks/]
    AG --> AH[OpenAIServiceMock.ts]
    A --> AI[.gitignore]
    A --> AJ[jest.config.js]
    A --> AK[package.json]
    A --> AL[tsconfig.json]
    A --> AM[README.md]
    A --> AN[/dist/]
```
