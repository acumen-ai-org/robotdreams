export default function DesignVoice() {
  return (
    <>
      <section className="mc-ongoing-section" aria-labelledby="voice-heading">
        <h2 id="voice-heading">Voice</h2>
        <ul className="mc-doc-list">
          <li>
            <strong>Sentence case</strong> everywhere; ALL CAPS only for tiny technical captions (the small uppercase
            labels naming groups and columns).
          </li>
          <li>
            <strong>Numbers are asserted in mono</strong>, words are not. A count, a version, a timestamp, an id — mono
            with tabular-nums. Prose never.
          </li>
          <li>
            <strong>Empty states say what and why</strong>, not just "no data":{" "}
            <em>"Nothing announced yet. `dream updates announce` tells every node a version is available."</em>
          </li>
          <li>
            <strong>Silence is an answer.</strong> A node that has not responded is counted and named, never omitted —
            and a node that declined is never painted as a failure.
          </li>
          <li>
            <strong>Provenance lines</strong> close every generated page: where the content came from, so the reader can
            check the page against its source.
          </li>
        </ul>
        <span className="mc-provenance">source: DESIGN.md — Typography · the docs pages' own conventions</span>
      </section>
    </>
  );
}
