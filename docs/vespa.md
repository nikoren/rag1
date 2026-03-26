# intuition

| #  | Concept            | When Defined | The Intuition                                                                                                                                              |
|----|--------------------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1  | Field Types        | Upfront      | Deciding if a piece of data is a "Word" (string), a "Number" (long), or a "Point in Space" (tensor). You can't change the "shape" of data easily once it's in the engine. |
| 2  | Index vs. Attribute| Upfront      | Index is for finding things (Search). Attribute is for sorting, math, and filtering. You usually want content to be an index and timestamp to be an attribute. |
| 3  | Tensor Dimensions  | Upfront      | Defining the size of your vector (e.g., x[384]). If you change your embedding model later, you have to update the schema and re-index.                       |
| 4  | HNSW (The Map)     | Upfront      | HNSW ("Hierarchical Navigable Small World") is the algorithm Vespa uses to make vector search extremely fast. Think of it as Vespa building a city map where each point is an embedding, and roads connect similar points for shortcuts. When you index data, you define HNSW settings so Vespa can organize these "roads" efficiently. As a result, Vespa can find the nearest neighbors of any vector in milliseconds—even among millions. You can also tune HNSW (like the number of links per node or how much memory vs. speed you want) up front in your schema. |
| 5  | Rank Profiles      | Upfront      | The "Scoring Script." You define the math (e.g., BM25+Closeness) here. It’s pre-compiled into machine code for speed.                                      |
| 6  | Parent-Child Links | Upfront      | The "Ancestry." You define which documents "belong" to others so you don't have to duplicate metadata (like book titles) on every chunk.                   |
| 7  | YQL (The Filter)   | Query Time   | The "Shopping List." This is where you tell Vespa: "Give me chunks from this book that mention this keyword or are near this vector."                      |
| 8  | User Vector        | Query Time   | The "Current Location." You generate the embedding of the user's question in Go and pass it in as a variable for the math to use.                          |
| 9  | Query Features     | Query Time   | The "Dials & Knobs." You can pass variables (like weight_vector: 0.8) to the rank profile to change how much you care about keywords vs. vectors on the fly. |
| 10 | Target Hits        | Query Time   | The "Safety Limit." You decide for each search how many "neighbors" Vespa should look at before it stops and calculates the final score.                    |

# Tensors

A tensor is a multidimensional array of numbers. In the world of RAG and Vespa, it is the mathematical fingerprint of your data.

### What it looks like

In the world of Machine Learning and Vespa, the word "dimension" is used in two different ways, which can be confusing:

**A Note About Embedding JSON Formats**

When querying some embedding APIs (for example, running:  

```
$ curl http://127.0.0.1:1234/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-embedding-all-minilm-l6-v2-embedding",
    "input": "Some text to embed"
  }'
```

), the response you get may look like this (heavily abbreviated for clarity):

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "embedding": [
        -0.0055,
        0.0284,
        0.0557,
        ... // hundreds more numbers ...
        -0.0003
      ],
      "index": 0
    }
  ],
  "model": "text-embedding-all-minilm-l6-v2-embedding",
  "usage": { ... }
}
```

Notice how the embedding itself is just a **flat array of numbers** (e.g., 384 values for a "384-dimensional" vector model).  
This is a *Rank 1* tensor—a one-dimensional list of floats—where each position is one of the model's output values.

---

**Understanding Vespa Tensors**

1. **Tensor Rank ("Shape"):**  
   This is what you specify in your Vespa schema and it determines data organization.
   - **Rank 1:** A simple list (array). A single axis, e.g., `x[384]`. Most RAG use cases use this shape.
   - **Rank 2:** Like a matrix (spreadsheet). Two axes needed, e.g., `x[10], y[10]`.
   - **Rank 3:** Like a cube. Three axes, e.g., `x[2], y[2], z[2]`.

   In Go, a 3-rank tensor (cube) might look like: `[][][]float32`.

2. **Vector Space Dimension ("Length"):**  
   When people say "384-dimensional embedding", it means a *Rank 1* tensor (a list of floats) that is 384 numbers long.  
   Vespa expresses this as `tensor<float>(x[384])`: just one axis `x`, but it holds 384 values.

*It’s important to know which “dimension” someone is referring to—shape (rank) or length (vector size).*

---

**Vespa Tensor JSON versus Flat Embeddings**

- **Embedding API (like above):**  
  Returns a flat array of numbers. Each position corresponds to the embedding’s index.

- **Vespa Tensor JSON (for attribute-valued `"tensor"` fields):**  
  Vespa uses a slightly more verbose representation for tensors, especially for higher ranks (or for sparse data).
  For 1D, it could look like:

  ```json
  {
    "embedding": {
      "cells": [
        { "address": { "x": "0" }, "value": -0.0055 },
        { "address": { "x": "1" }, "value": 0.0284 },
        // ...more values...
        { "address": { "x": "383" }, "value": -0.0003 }
      ]
    }
  }
  ```

  For higher-rank tensors, you’ll see more coordinates in each `"address"`:

  ```json
  {
    "cube": {
      "cells": [
        { "address": { "x": "0", "y": "0", "z": "0" }, "value": 0.12 },
        { "address": { "x": "1", "y": "0", "z": "1" }, "value": -0.18 }
        // etc.
      ]
    }
  }
  ```

- **Schema Examples:**  
  - Flat embedding: `tensor<float>(x[384])`
  - 3D: `tensor<float>(x[2],y[2],z[2])`

**Summary:**  
If your embedding API gives you back a flat array, you’ll typically convert it to Vespa’s “cells” format (see above) if you need to send it as a tensor JSON to Vespa. Remember, "Rank" is the number of axes; "dimensions" can also mean the length of your vector!

### What it represents

It is a coordinate on a high-dimensional map. If your model has 384 dimensions, it is placing your text at a specific intersection of 384 different "concept" axes.

### The "Must-Knows"

The Dimension Lock: If your schema defines x[384], you must send exactly 384 numbers. If you switch to a 768-dim model (like Nomic), you have to update the schema and re-index all data.

Indexed vs. Mapped:

Indexed (x[384]): Like a Go array. Best for dense vectors and "nearest neighbor" searches.

Mapped (x{}): Like a Go map[string]float. Best for sparse data or "tagging" (e.g., {"funny": 0.9, "tech": 0.2}).

Distance Metric: You must specify how Vespa calculates "closeness" (e.g., distance-metric: angular). This determines if two points are "similar."

Cell Type: float is standard, but bfloat16 or int8 can be used to cut your RAM costs in half if your embedding model supports quantization.

# Profile ranking

A **Rank Profile** is a named mathematical recipe in your schema that tells Vespa how to calculate a "relevance score" for each document. While the query (`YQL`) finds the candidates, the rank profile decides who wins the top spot.

### 1. What they are & When to use

Think of them as **Scoring Scripts**. You use them whenever you need to sort results by something other than a simple timestamp. In a RAG application, you’ll use them to balance "exact word matches" against "vector similarity."

### 2. The Top 3 Functions Most Users Use

- **`bm25(field)`**: The industry standard for keyword matching. It rewards documents where the search terms appear frequently but aren't common across the whole library.
- **`closeness(tensor_field)`**: The "semantic" score. It measures how physically close your query vector is to the document's vector.
- **`freshness(timestamp_field)`**: A linear decay function that gives a boost to more recent data (crucial for Slack/Zoom logs).

| Search Strategy (The "Project")  | Primary Function Used (The "Tool")       | The Goal                                            |
|----------------------------------|------------------------------------------|-----------------------------------------------------|
| Keyword-Only                     | bm25(content)                            | Find exact words, names, or error codes.            |
| Vector-Only                      | closeness(embedding)                     | Find general meanings or broad concepts.            |
| Hybrid (RAG Standard)            | bm25() + closeness()                     | The best of both worlds—meaning + exactness.        |
| Recency-Boosted-metadata weighted| ... + freshness(time)                    | Make sure the newest info stays at the top.         |

### 3. What You Absolutely Must Know

- **Phases:** Use `first-phase` for a quick sort of thousands of documents and `second-phase` for a deep-dive calculation on just the top 100–500.

- **Inheritance:** You can make one profile inherit from another.

    ```text
    rank-profile base { ... }
    rank-profile experimental inherits base { ... }
    ```

- **Query Time Activation:** You switch profiles by sending `&ranking.profile=my_profile` in your HTTP request. You don't need to change your data, just the query parameter.

- **Inputs:** You must define `inputs` in the profile to receive tensors or variables from your Go code.

---

### Comparison: Upfront vs. Query Time

| Feature | Defined In Schema | Defined In Query |
| :--- | :--- | :--- |
| **The Formula** | Yes (The "How") | No |
| **The Weights** | Usually (e.g., $0.7$) | Optional (via variables) |
| **The Profile Name** | Yes | Yes (The "Which") |
| **The Limit (Hits)** | No | Yes |

# NHSW

stands for **Hierarchical Navigable Small World**. it is an internal **graph-based data structure**  similar to Heap or B-tree..
While a **Heap** maintains order for priority (finding the smallest/largest), and a **B-Tree** maintains order for ranges, **HNSW** maintains "proximity" in a high-dimensional space. It functions like a **Skip List** for graphs; it has a "top floor" with a few long-distance jumps and a "ground floor" with many short-distance connections.

---

### Standard HNSW Settings (for 384-dim Vectors)

For a typical RAG application using **MiniLM** (384 dimensions) on a few million documents, these settings are the industry "sweet spot."

| Parameter | Recommended Value | Why? |
| :--- | :--- | :--- |
| **`max-links-per-node`** | `16` | Higher = more accurate but uses significantly more RAM. `16` is the standard for 384 dims. |
| **`neighbors-to-explore-at-insert`** | `200` | Controls the "quality" of the highway during indexing. `200` ensures a very high recall (accuracy) for RAG. |
| **`distance-metric`** | `angular` | This is the standard for most modern embedding models (effectively Cosine Similarity). |

**In your Schema:**

```text
index {
    hnsw {
        max-links-per-node: 16
        neighbors-to-explore-at-insert: 200
    }
}
```

---

**How it Works:**
It mimics a "Small World" network (like LinkedIn). You can reach anyone in a few hops. At search time, Vespa starts at the top layer, making massive leaps across the "concept map." As it gets closer to your query's coordinates, it drops down a layer to refine the search, eventually finding the exact neighbors on the bottom layer.

**Key Implementation Details:**

- **Definition:** Defined **upfront** in the `.sd` file inside the tensor field.
- **Maintenance:** Managed **internally** by Vespa. When you `POST` a document, Vespa automatically inserts it into the graph.
- **Querying:** Triggered automatically via the `nearestNeighbor()` `YQL` operator.
- **The Trade-off:** HNSW lives entirely in **RAM**. Your primary goal is balancing **Recall** (finding the *true* best match) against **Memory Cost**.

**The Bottom Line:**
For RAG, HNSW is what prevents your "Company Brain" from getting slower as you add more books. It turns an `O(N)` search into an `OlogN` search, taking you from seconds to **milliseconds** even as your library grows from thousands to millions of messages.

Estimating the RAM for HNSW is essentially calculating the cost of the **Vector Storage** plus the **Link Infrastructure**.

### The Estimation Path

1. **Count Your Chunks ($N$):** A 100MB text file is ~100 million characters. With a standard RAG chunk size of 1,000 characters (approx. 250 tokens), you get **100,000 chunks**.
2. **Vector Cost:** Each chunk has 384 `float32` numbers (4 bytes each).
    $100,000 \times 384 \times 4 \text{ bytes} \approx \mathbf{153.6 \text{ MB}}$
3. **HNSW Graph Cost:** Each chunk stores connections to its neighbors. With `max-links-per-node: 16`, each node stores roughly 16 links (8 bytes each for internal pointers).
    $100,000 \times 16 \times 8 \text{ bytes} \approx \mathbf{12.8 \text{ MB}}$
4. **Total Estimate:** $153.6 \text{ MB} + 12.8 \text{ MB} = \mathbf{166.4 \text{ MB}}$

### Summary in < 200 Words

To estimate HNSW size, follow this formula:
**Total RAM $\approx (N \times D \times 4) + (N \times M \times 8)$**

- **$N$ (Number of Chunks):** For 100MB of text, expect ~100k chunks.
- **$D$ (Dimensions):** Your model's output (384).
- **$M$ (Max Links):** Your schema setting (16).

In this 100MB book example, your "Brain" needs about **166MB of RAM**. This fits easily on your MacBook Pro, but if you scale to 1,000 books, you'd need ~166GB, which is where you'd start looking at **8-bit quantization** (changing the `4` in the formula to a `1`) to cut costs by 75%.

## Quantization

Quantization is like switching from a **high-precision laser** to a **standard ruler**.

Normally, each coordinate in your 384-dim vector is a `float32` (4 bytes). This provides extreme precision, but it’s often overkill for finding "neighbors." Quantization squashes that 4-byte float into a single **1-byte integer** (int8).

**The Intuitive Analogy:**
Imagine trying to find a house in a city. You don’t need the GPS coordinates down to the *millimeter* (float32); knowing the *meter* (int8) is plenty to get you to the right front door. You sacrifice "perfect" accuracy for massive efficiency.

**The Benefits:**

- **Memory:** Your RAM usage immediately drops by 75%. That 166MB book index shrinks to roughly **50MB**.
- **Speed:** Modern CPUs can compare 1-byte integers significantly faster than 4-byte floats using SIMD instructions.
- **The Trade-off:** You lose a tiny bit of "Recall" (accuracy). In RAG, this is usually negligible—the semantic meaning remains intact even if the coordinates are slightly "fuzzy."

In your Vespa schema, you simply define the tensor as `tensor<int8>(x[384])`.

Would you like me to show you the specific **HNSW** and **Rank Profile** changes needed to handle `int8` data?

### example

```schema knowledge_chunk {
    document knowledge_chunk {
        field parent_ref type reference<source_metadata> {
            indexing: summary | attribute
        }
        field sequence_index type int {
            indexing: summary | attribute
        }
        field chunk_id type string {
            indexing: summary | attribute
        }
        field chunk_hash type string {
            indexing: summary | attribute
        }
        field indexed_at type long {
            indexing: summary | attribute
        }
        field text_content type string {
            indexing: summary | index
        }
        field embedding type tensor<float>(x[384]) {
            indexing: attribute | index
            attribute { distance-metric: angular }
        }
    }

    import field parent_ref.url as url {}
    import field parent_ref.source_type as source_type {}

    rank-profile hybrid_search {
        inputs {
            query(user_vector) tensor<float>(x[384])
        }
        first-phase {
            expression: closeness(field, embedding) + bm25(text_content)
        }
    }
}
```

In your current schema, HNSW is actually **active**, but it is operating on **implicit defaults**.

The "magic" happens in this line: `indexing: attribute | index`. Because you included the word `index` for a tensor field, Vespa knows it must build a HNSW graph. If you omitted `index` and only had `attribute`, Vespa would switch to **Exact Search (Brute Force)**.

### 1. What happens if HNSW is not defined at all?

If you remove the `index` keyword from your `indexing` script:

- **Brute Force:** Vespa will perform an "Exact" search. For every query, it will manually calculate the distance between your input and *every single document* in your index.
- **Accuracy:** 100%. You will always get the mathematically perfect nearest neighbors.
- **Performance:** It scales linearly ($O(N)$). With 10,000 documents, it's fast. With 10 million documents, your search latency will spike from milliseconds to seconds, making your RAG app unusable.

---

### 2. Your Schema with Explicit HNSW

Even though it's working now, most developers define it explicitly so they can tune the performance/memory trade-off as their book library grows.

Here is how your `embedding` field looks with the explicit configuration:

```text
field embedding type tensor<float>(x[384]) {
    indexing: attribute | index
    attribute { 
        distance-metric: angular 
    }
    index {
        hnsw {
            max-links-per-node: 16
            neighbors-to-explore-at-insert: 200
        }
    }
}
```

### 3. The Implicit Defaults

When you leave the `index { hnsw { ... } }` block out (like in your current code), Vespa uses these standard values:

- **`max-links-per-node`**: **16**
- **`neighbors-to-explore-at-insert`**: **200**

### What you should know

Since you are working with 384-dimensional vectors, the default `16` is a very solid choice. However, as you scale toward millions of chunks, you might find that explicitly setting `neighbors-to-explore-at-insert` to a higher number (like `400`) improves your **Recall** (the quality of the chunks you get back for RAG) at the cost of slightly slower ingestion speed.

---

### Summary of your current state

- **Is it implicit?** Yes, because you have `indexing: index`.

- **Is it fast?** Yes, you are already getting the speed benefits of HNSW.
- **Should you change it?** Keep it as is for now. Only add the explicit block if you notice your RAG results aren't "accurate" enough or if you want to experiment with `int8` quantization to save RAM.

## QUERRYING

Vespa querying is basically writing a specialized "Shopping List" (YQL) and then twisting "Dials" (Query Features) to get the perfect result.

---

### 1. The Simple YQL (Basic Retrieval)

At its simplest, you are asking for documents that match a criteria.

- **Keyword:** `select * from sources * where text_content contains "pagination";`
- **Vector:** `select * from sources * where {targetHits:10}nearestNeighbor(embedding, user_vector);`

### 2. The Hybrid Query (The RAG Standard)

This is where you combine the "Proofreader" and the "Mind Reader." You use the `OR` operator so Vespa grabs candidates from both worlds.

```sql
select * from sources * where 
  userInput(@user_text) or ({targetHits:20}nearestNeighbor(embedding, user_vector));
```

### 3. Accuracy & The "Safety Limit" (`targetHits`)

Vector search is **approximate** (ANN) by default to stay fast.

- **`targetHits`**: This is your "Safety Limit." It tells Vespa: "Don't stop searching the HNSW highway until you've found at least 20 solid candidates."
- **The Trade-off:** Higher `targetHits` = higher accuracy (Recall) but slightly slower search. For RAG, 10–50 is the sweet spot.

### 4. Dials & Knobs (Query Features)

You can pass variables from Go that your Rank Profile uses for math.

- **Example:** In Go, you send `input.query(alpha) = 0.8`.
- **In Schema:** `expression: query(alpha) * closeness(embedding)`.
This lets you tune your hybrid "blend" without re-deploying your schema.

### 5. Managing the Flood (Pagination)

Vespa handles pagination via `hits` and `offset` parameters in the JSON body (not the YQL string).

- **`hits`**: How many results to return (e.g., 5 for an LLM context).
- **`offset`**: Where to start (e.g., `offset: 10` skips the first 10).
- **Pro Tip:** For RAG, you rarely need deep pagination. If the "truth" isn't in the top 10 chunks, your embedding model or chunking strategy is likely the issue, not the pagination.

### Summary Table

| Feature | Concept | JSON Parameter |
| :--- | :--- | :--- |
| **YQL** | The Logic | `"yql": "select...where..."` |
| **User Vector** | The Meaning | `"input.query(user_vector)": [...]` |
| **Target Hits** | Accuracy | Inside the `nearestNeighbor()` operator |
| **Hits/Offset** | Pagination | `"hits": 5, "offset": 0` |

**The Goal:** Use YQL to cast a wide net (High Recall) and Rank Profiles to narrow it down to the "Perfect 5" (High Precision).
