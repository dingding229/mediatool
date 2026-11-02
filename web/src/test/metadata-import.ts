// I41b approves only this display-helper import. Historic executable-action hashes remain unchanged.
export const stripMetadataImport=(source:string)=>source.replace(/^import \{joinMetadata\} from ['"]\.\.\/presentation\/metadata['"];\r?\n(?:\r?\n)?/gm,'');
