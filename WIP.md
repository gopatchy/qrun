# WIP: Iterative buildCells rewrite

## What's done
- Block has `weight uint64`, `triggers []*Trigger`, `topRow int` fields
- TriggerSource and TriggerTarget have `block *Block` pointer fields
- `linkTriggers()` resolves string IDs to block pointers, populates `block.triggers`
- `computeWeights()` sets weight = `(cue index) << 32` + 0 if cue-ended, 1 otherwise
  - Higher weight = more likely to be moved
  - Later cues = higher weight
- `buildCells()` is rewritten to be iterative:
  1. Init: cues get sequential topRow, non-cues start at 0
  2. Loop (trigger pass): move target blocks so trigger source row >= target hook row
  3. Loop (overlap pass): sort same-track blocks by (topRow, weight), push overlaps to prev end + 1
  4. After convergence: move each cue to max row of its triggered cells
  5. `emitCells()` creates actual TimelineCell objects from computed topRows
- Test uses `BuildTimelineDebug` with testWriter for debug output

## What's broken
- Cue track comes out unordered after the cue-move step (cues placed at their highest triggered cell row, which scrambles scene order)
- Gap padding in emitCells skips cue track (`if block.Type != "cue"`) so cue cell rows don't match topRow
- The +1 gap between blocks may be wrong (was +2, changed to +1)
- assignRows still can't converge after buildCells — off-by-one misalignments between cue rows and target rows

## Key design decisions
- topRow coordinate space should include gap cells (emitCells just places cells at topRow)
- blockHeight: cue=0 (own track, can overlap), other=4
- Gap between same-track blocks = 1 row (chain or gap cell)
- Cross-track alignment still handled by assignRows solver after buildCells

## Next steps
- Fix cue track: need gap padding on cue track too so cell rows match topRow
- May need to rethink cue-move step — moving cues to highest triggered row scrambles cue order
- Verify topRow math matches actual emitted cell rows end-to-end
