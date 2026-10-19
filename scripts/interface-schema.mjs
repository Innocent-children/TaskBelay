/**
 * Build documentation schemas from recorded producer examples. Input contracts
 * remain explicit; output records share the exact captured JSON shape.
 */
export function mergeSchema(left, right) {
  if (!left) return right;
  if (!right) return left;
  const types = [...new Set([...(Array.isArray(left.type) ? left.type : [left.type]), ...(Array.isArray(right.type) ? right.type : [right.type])])].filter(Boolean);
  const value = { ...left, ...(types.length ? { type: types.length === 1 ? types[0] : types } : {}) };
  if (types.includes('object')) {
    value.properties = { ...left.properties };
    for (const [name, child] of Object.entries(right.properties ?? {})) value.properties[name] = mergeSchema(value.properties[name], child);
    value.required = (left.required ?? []).filter(name => (right.required ?? []).includes(name));
    value.additionalProperties = left.additionalProperties === false && right.additionalProperties === false ? false : (left.additionalProperties ?? right.additionalProperties ?? true);
  }
  if (types.includes('array')) value.items = mergeSchema(left.items, right.items);
  return value;
}

export function exampleSchema(value, descriptions = {}, field = '') {
  const description = descriptions[field] ?? (field ? `Returned ${field}; see the method's field notes and complete examples.` : 'Complete method result.');
  if (['input_schema','output_schema'].includes(field)) return { type:'object',description:'Complete JSON Schema for the selected method. Interpret with JSON Schema rules; it is data, not an operation request.',additionalProperties:true };
  if (['handoff','snapshot','host_request','session','confirmation'].includes(field)) return {type:['object','null'],description:description+' Retain this producer-owned object unchanged; do not reconstruct it or infer that an external action executed.',additionalProperties:true};
  if (['source_dirty_paths','source_dirty_paths_truncated'].includes(field)) return {type:'object',description,additionalProperties:field==='source_dirty_paths'?{type:'array',items:{type:'string',description:'Relative dirty path in this repository.'}}:{type:'boolean'}};
  if (field === 'changed' && value === null) return {type: ['boolean','null'], description};
  if (field === 'failed_action') return {type: ['string','null'], description};
  if (value === null) return { type: ['string','null'], description };
  if (Array.isArray(value)) return { type: 'array', description, items: value.reduce((schema, item) => mergeSchema(schema, exampleSchema(item, descriptions, field)), undefined) ?? {description:description+' Empty in this example; state-dependent records are described by the owning method.'} };
  if (typeof value === 'object') return { type: 'object', description, additionalProperties: false, properties: Object.fromEntries(Object.entries(value).map(([name, child]) => [name, exampleSchema(child, descriptions, name)])), required: Object.keys(value) };
  return { type: typeof value === 'number' ? Number.isInteger(value) ? 'integer' : 'number' : typeof value, description };
}

export function inputObject(properties, optional = []) {
  return { type: 'object', additionalProperties: false, properties, required: Object.keys(properties).filter(name => !optional.includes(name)) };
}
export const field = (description, type = 'string', extra = {}) => ({ type, description, ...extra });
