# eagle

This app helps users improve their English skills by providing a platform for language practice. It displays random Japanese sentences, and users can input their English translations.

## Technical Stack

- Backend: Go
- Frontend: Next.js
- API: REST
- Database: MySQL

---

## Database Table Spec

### `sentences` Table

| Column Name | Type     | Constraints | Description                 |
| ----------- | -------- | ----------- | --------------------------- |
| id          | INTEGER  | PRIMARY KEY | Unique ID                   |
| japanese    | TEXT     | NOT NULL    | Japanese sentence           |
| english     | TEXT     | NOT NULL    | Correct English translation |
| page        | TEXT     | NOT NULL    | Page number                 |
| created_at  | DATETIME | NOT NULL    | Timestamp                   |
| updated_at  | DATETIME | NOT NULL    | Timestamp                   |

### `answer_histories` Table

| Column Name      | Type     | Constraints | Description                                                          |
| ---------------- | -------- | ----------- | -------------------------------------------------------------------- |
| id               | INTEGER  | PRIMARY KEY | Unique ID                                                            |
| sentence_id      | INTEGER  | FOREIGN KEY | Reference to `sentence.id`                                           |
| is_correct       | BOOLEAN  | NOT NULL    | Whether the answer is correct                                        |
| incorrect_answer | TEXT     | NOT NULL    | The user’s English answer. If correct, this will be an empty string. |
| created_at       | DATETIME | NOT NULL    | Timestamp                                                            |
| updated_at       | DATETIME | NOT NULL    | Timestamp                                                            |

---

## API List

| Method | Path                 | Description                      |
| ------ | -------------------- | -------------------------------- |
| GET    | /api/sentence/random | Get a random Japanese sentence   |
| POST   | /api/answer/check    | Check user's English translation |

---

## API Interface Spec

### Get Random Japanese Sentence

**GET** `/api/sentence/random`

**Request:**
_No request body_

**Response:**

| Field      | Type    | Description                 |
| ---------- | ------- | --------------------------- |
| id         | INTEGER | Sentence unique ID          |
| japanese   | TEXT    | Japanese sentence           |
| english    | TEXT    | Correct English translation |
| page       | TEXT    | Page number                 |
| created_at | STRING  | ISO 8601 Timestamp          |
| updated_at | STRING  | ISO 8601 Timestamp          |

```json
{
    "id": 1,
    "japanese": "時間がありません。",
    "english": "I don't have time.",
    "page": "12",
    "created_at": "2024-06-28T10:00:00Z",
    "updated_at": "2024-06-28T10:00:00Z"
}
```

### Check User’s English Translation

**POST** `/api/answer/check`

**Request:**

| Field       | Type    | Description            |
| ----------- | ------- | ---------------------- |
| sentence_id | INTEGER | Sentence unique ID     |
| user_answer | TEXT    | User's answer sentence |

```json
{
    "sentence_id": 1,
    "user_answer": "I don't have time."
}
```

**Response:**

| Field1         | Field2           | Type    | Description                                |
| -------------- | ---------------- | ------- | ------------------------------------------ |
| is_correct     | —                | BOOLEAN | Whether the answer is correct              |
| correct_answer | —                | TEXT    | Correct English translation                |
| histories      | -                | ARRAY   | Answer histories                           |
|                | id               | INTEGER | Answer history record ID                   |
|                | incorrect_answer | TEXT    | Previously submitted incorrect answer      |
|                | created_at       | STRING  | ISO 8601 Timestamp of incorrect submission |

```json
{
    "is_correct": false,
    "correct_answer": "I don't have time.",
    "histories": [
        {
            "id": 1001,
            "incorrect_answer": "I have no time.",
            "created_at": "2024-06-27T15:21:30Z"
        },
        {
            "id": 1020,
            "incorrect_answer": "There is no time.",
            "created_at": "2024-06-28T09:55:12Z"
        },
        {
            "id": 1050,
            "incorrect_answer": "I don't have times.",
            "created_at": "2024-06-28T10:08:41Z"
        }
    ]
}
```
